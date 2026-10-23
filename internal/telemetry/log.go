package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"strings"
	"sync"
)

// MaxText is the longest string value written to a log; longer text is cut.
// This is a backstop only: licensed content is omitted whatever its length.
const MaxText = 256

const redacted = "[REDACTED]"

// Keys whose values are secrets, and keys whose values are licensed or
// third-party content, whatever their length. Matching ignores case and the
// characters "-", "_" and ".".
var secretKeys = []string{"token", "password", "passwd", "secret", "authorization", "cookie", "apikey", "dsn",
	"databaseurl", "useragent", "credential", "privatekey", "bottoken"}
var contentKeys = []string{"body", "content", "document", "payload", "companyfacts", "submissions", "source", "input", "stdin"}

// Credential shapes replaced even when not registered: Telegram bot tokens,
// GitHub tokens, AWS access key IDs, bearer values and URL passwords.
var credentialShapes = []*regexp.Regexp{
	regexp.MustCompile(`\b\d{6,12}:[A-Za-z0-9_-]{30,}\b`),
	regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{30,}\b`),
	regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/-]+=*`),
	regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.-]*://[^:/@\s]+:)[^@\s]+@`),
}

// Licensed marks content that must never be logged, such as an SEC source
// document; a log shows only its size.
type Licensed []byte

func (l Licensed) omitted() string { return fmt.Sprintf("[omitted: %d bytes]", len(l)) }

// LogValue implements slog.LogValuer.
func (l Licensed) LogValue() slog.Value { return slog.StringValue(l.omitted()) }

// MarshalJSON keeps the content out when the marker sits inside a list, map or
// struct that a log line turns into JSON (a plain []byte would become base64).
func (l Licensed) MarshalJSON() ([]byte, error) { return json.Marshal(l.omitted()) }

// Format keeps the content out of fmt output for every verb, for example an
// error message that wraps the marker.
func (l Licensed) Format(f fmt.State, _ rune) { _, _ = io.WriteString(f, l.omitted()) }

// Redactor removes secrets from log values.
type Redactor struct {
	mu     sync.RWMutex
	values []string
}

// Add registers secret values to remove wherever they appear. Values shorter
// than 8 bytes are ignored, so short common words are never blanked.
func (r *Redactor) Add(values ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, v := range values {
		if len(v) >= 8 {
			r.values = append(r.values, v)
		}
	}
}

// Clean replaces registered secrets and credential shapes, then cuts long text.
func (r *Redactor) Clean(s string) string {
	r.mu.RLock()
	for _, v := range r.values {
		s = strings.ReplaceAll(s, v, redacted)
	}
	r.mu.RUnlock()
	for i, shape := range credentialShapes {
		if i == len(credentialShapes)-1 {
			s = shape.ReplaceAllString(s, "${1}"+redacted+"@")
		} else {
			s = shape.ReplaceAllString(s, redacted)
		}
	}
	if len(s) > MaxText {
		s = fmt.Sprintf("%s…[cut: %d bytes]", s[:MaxText], len(s))
	}
	return s
}

func normalKey(key string) string {
	return strings.NewReplacer("-", "", "_", "", ".", "").Replace(strings.ToLower(key))
}

func keyIn(key string, list []string) bool {
	k := normalKey(key)
	for _, word := range list {
		if strings.Contains(k, word) {
			return true
		}
	}
	return false
}

// clean returns a safe copy of one attribute.
func (r *Redactor) clean(a slog.Attr) slog.Attr {
	raw := a.Value.Any() // before Resolve, which turns Licensed into its marker text
	a.Value = a.Value.Resolve()
	switch {
	case keyIn(a.Key, secretKeys):
		return slog.String(a.Key, redacted)
	case keyIn(a.Key, contentKeys):
		if a.Value.Kind() == slog.KindGroup {
			return slog.String(a.Key, fmt.Sprintf("[omitted: group of %d fields]", len(a.Value.Group())))
		}
		if l, ok := raw.(Licensed); ok {
			return slog.String(a.Key, l.omitted())
		}
		return slog.String(a.Key, fmt.Sprintf("[omitted: %d bytes]", len(a.Value.String())))
	}
	switch a.Value.Kind() {
	case slog.KindString:
		return slog.String(a.Key, r.Clean(a.Value.String()))
	case slog.KindGroup:
		attrs := a.Value.Group()
		out := make([]any, len(attrs))
		for i, inner := range attrs {
			out[i] = r.clean(inner)
		}
		return slog.Group(a.Key, out...)
	case slog.KindAny:
		if err, ok := a.Value.Any().(error); ok {
			return slog.String(a.Key, r.Clean(err.Error()))
		}
		// Every other value (typed maps, typed slices, structs, nested mixes) is
		// first turned into plain JSON maps and lists, so the key rules reach
		// every level. A value that cannot be encoded is logged as its type only.
		text, err := json.Marshal(a.Value.Any())
		if err != nil {
			return slog.String(a.Key, fmt.Sprintf("[unloggable %T]", a.Value.Any()))
		}
		var generic any
		if err = json.Unmarshal(text, &generic); err != nil {
			return slog.String(a.Key, fmt.Sprintf("[unloggable %T]", a.Value.Any()))
		}
		return slog.Any(a.Key, r.cleanValue(generic))
	}
	return a
}

// cleanMap applies the same key and value rules inside nested maps and lists.
func (r *Redactor) cleanMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		switch {
		case keyIn(k, secretKeys):
			out[k] = redacted
		case keyIn(k, contentKeys):
			text, _ := json.Marshal(v)
			out[k] = fmt.Sprintf("[omitted: %d bytes]", len(text))
		default:
			out[k] = r.cleanValue(v)
		}
	}
	return out
}

func (r *Redactor) cleanValue(v any) any {
	switch x := v.(type) {
	case string:
		return r.Clean(x)
	case map[string]any:
		return r.cleanMap(x)
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = r.cleanValue(e)
		}
		return out
	}
	return v
}

// How sensitive an open group is: inside a secret-keyed group every value is
// redacted, inside a content-keyed group every value is omitted.
type scope int

const (
	ordinary scope = iota
	content
	secret
)

func scopeOf(name string) scope {
	switch {
	case keyIn(name, secretKeys):
		return secret
	case keyIn(name, contentKeys):
		return content
	}
	return ordinary
}

// frame is one open group and the attributes added inside it.
type frame struct {
	name  string
	attrs []slog.Attr
}

// Handler is an slog.Handler that cleans every attribute and adds the trace
// and span IDs from the context, always at the top level. It keeps the group
// nesting itself (the inner handler never sees WithGroup), so a group named
// like a secret or like content redacts or omits everything inside it.
type Handler struct {
	inner    slog.Handler
	redactor *Redactor
	frames   []frame // frames[0] is the top level; later entries are open groups
	scope    scope   // the most sensitive open group
}

// NewHandler wraps inner; all attributes pass through r.
func NewHandler(inner slog.Handler, r *Redactor) *Handler {
	return &Handler{inner: inner, redactor: r, frames: []frame{{}}}
}

// NewLogger writes JSON lines to w.
func NewLogger(w io.Writer, r *Redactor, level slog.Level) *slog.Logger {
	return slog.New(NewHandler(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level}), r))
}

func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

// cleanIn cleans a within the handler's open groups.
func (h *Handler) cleanIn(a slog.Attr) slog.Attr {
	if a.Key == "" && a.Value.Kind() == slog.KindAny && a.Value.Any() == nil {
		return a // an empty attribute, which handlers ignore
	}
	switch h.scope {
	case secret:
		return mask(a, redacted)
	case content:
		return mask(a, "[omitted: inside a content group]")
	}
	return h.redactor.clean(a)
}

// mask keeps a's keys, including those of nested groups, and replaces every
// value with text.
func mask(a slog.Attr, text string) slog.Attr {
	a.Value = a.Value.Resolve()
	if a.Value.Kind() != slog.KindGroup {
		return slog.String(a.Key, text)
	}
	attrs := a.Value.Group()
	out := make([]slog.Attr, len(attrs))
	for i, inner := range attrs {
		out[i] = mask(inner, text)
	}
	return slog.Attr{Key: a.Key, Value: slog.GroupValue(out...)}
}

func (h *Handler) Handle(ctx context.Context, rec slog.Record) error {
	out := slog.NewRecord(rec.Time, rec.Level, h.redactor.Clean(rec.Message), rec.PC)
	if t, ok := FromContext(ctx); ok {
		out.AddAttrs(slog.String("trace_id", t.TraceID), slog.String("span_id", t.SpanID))
	}
	last := len(h.frames) - 1
	innermost := append([]slog.Attr(nil), h.frames[last].attrs...)
	rec.Attrs(func(a slog.Attr) bool {
		innermost = append(innermost, h.cleanIn(a))
		return true
	})
	// Rebuild the nesting from the innermost group outwards.
	for i := last; i > 0; i-- {
		group := slog.Attr{Key: h.frames[i].name, Value: slog.GroupValue(innermost...)}
		innermost = append(append([]slog.Attr(nil), h.frames[i-1].attrs...), group)
	}
	out.AddAttrs(innermost...)
	return h.inner.Handle(ctx, out)
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := h.clone()
	last := len(next.frames) - 1
	for _, a := range attrs {
		next.frames[last].attrs = append(next.frames[last].attrs, h.cleanIn(a))
	}
	return next
}

func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	next := h.clone()
	next.frames = append(next.frames, frame{name: name})
	if s := scopeOf(name); s > next.scope {
		next.scope = s
	}
	return next
}

func (h *Handler) clone() *Handler {
	frames := make([]frame, len(h.frames))
	for i, f := range h.frames {
		frames[i] = frame{name: f.name, attrs: append([]slog.Attr(nil), f.attrs...)}
	}
	return &Handler{inner: h.inner, redactor: h.redactor, frames: frames, scope: h.scope}
}
