// Package contracts reads, validates and writes the JSON payloads defined in
// the repository's contracts/ folder. It uses the standard library only.
//
// Parse is strict on purpose, so Go and Python give the same verdict for the
// same bytes: valid UTF-8 only, no duplicate object keys, no unpaired UTF-16
// surrogates, no NaN or Infinity, nothing after the value, at most 64 levels
// of nesting, and bounded size (MaxBytes, MaxNodes, MaxNumberLength). Numbers
// keep their exact text (Number); the schemas allow only integers, so a decimal
// sent as a JSON number fails validation.
package contracts

import (
	"bytes"
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// Number is a JSON number exactly as written.
type Number string

var integerText = regexp.MustCompile(`^-?(0|[1-9][0-9]*)$`)

// Integer returns the value when the text is exactly a JSON integer: no
// fraction, exponent, plus sign or leading zero. Values built outside Parse
// are held to the same syntax.
func (n Number) Integer() (*big.Int, bool) {
	s := string(n)
	if len(s) > MaxNumberLength || !integerText.MatchString(s) {
		return nil, false
	}
	return new(big.Int).SetString(s, 10)
}

// Input limits, the same in Python. They bound the work of one Parse: the
// largest payload is an SEC parser request (two original documents of at most
// 16 MiB each).
const (
	MaxBytes        = 48 << 20
	MaxNodes        = 4_000_000
	MaxNumberLength = 128
	maxDepth        = 64
)

// ParseError reports why bytes are not acceptable JSON.
type ParseError struct {
	Offset  int
	Message string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("malformed JSON at byte %d: %s", e.Offset, e.Message)
}

// Parse decodes one JSON value into map[string]any, []any, string, Number,
// bool or nil.
func Parse(raw []byte) (any, error) {
	if len(raw) > MaxBytes {
		return nil, &ParseError{0, "input larger than MaxBytes"}
	}
	if !utf8.Valid(raw) {
		return nil, &ParseError{0, "invalid UTF-8"}
	}
	p := &parser{b: raw}
	p.space()
	v, err := p.value(0)
	if err != nil {
		return nil, err
	}
	p.space()
	if p.i != len(p.b) {
		return nil, p.fail("data after the JSON value")
	}
	return v, nil
}

type parser struct {
	b     []byte
	i     int
	nodes int
}

func (p *parser) fail(msg string) error { return &ParseError{p.i, msg} }

func (p *parser) space() {
	for p.i < len(p.b) {
		switch p.b[p.i] {
		case ' ', '\t', '\n', '\r':
			p.i++
		default:
			return
		}
	}
}

func (p *parser) value(depth int) (any, error) {
	if depth > maxDepth {
		return nil, p.fail("nesting deeper than 64 levels")
	}
	if p.nodes++; p.nodes > MaxNodes {
		return nil, p.fail("more than MaxNodes values")
	}
	if p.i >= len(p.b) {
		return nil, p.fail("unexpected end of input")
	}
	switch c := p.b[p.i]; {
	case c == '{':
		return p.object(depth)
	case c == '[':
		return p.array(depth)
	case c == '"':
		return p.str()
	case c == 't':
		return p.literal("true", true)
	case c == 'f':
		return p.literal("false", false)
	case c == 'n':
		return p.literal("null", nil)
	case c == '-' || (c >= '0' && c <= '9'):
		return p.number()
	}
	return nil, p.fail("unexpected character")
}

func (p *parser) literal(word string, v any) (any, error) {
	if !bytes.HasPrefix(p.b[p.i:], []byte(word)) {
		return nil, p.fail("invalid literal")
	}
	p.i += len(word)
	return v, nil
}

func (p *parser) object(depth int) (any, error) {
	p.i++ // {
	out := map[string]any{}
	p.space()
	if p.i < len(p.b) && p.b[p.i] == '}' {
		p.i++
		return out, nil
	}
	for {
		p.space()
		if p.i >= len(p.b) || p.b[p.i] != '"' {
			return nil, p.fail("object key must be a string")
		}
		start := p.i
		key, err := p.str()
		if err != nil {
			return nil, err
		}
		if _, dup := out[key]; dup {
			return nil, &ParseError{start, "duplicate object key"}
		}
		p.space()
		if p.i >= len(p.b) || p.b[p.i] != ':' {
			return nil, p.fail("expected ':'")
		}
		p.i++
		p.space()
		v, err := p.value(depth + 1)
		if err != nil {
			return nil, err
		}
		out[key] = v
		p.space()
		if p.i >= len(p.b) {
			return nil, p.fail("unexpected end of object")
		}
		if p.b[p.i] == ',' {
			p.i++
			continue
		}
		if p.b[p.i] == '}' {
			p.i++
			return out, nil
		}
		return nil, p.fail("expected ',' or '}'")
	}
}

func (p *parser) array(depth int) (any, error) {
	p.i++ // [
	out := []any{}
	p.space()
	if p.i < len(p.b) && p.b[p.i] == ']' {
		p.i++
		return out, nil
	}
	for {
		p.space()
		v, err := p.value(depth + 1)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
		p.space()
		if p.i >= len(p.b) {
			return nil, p.fail("unexpected end of array")
		}
		if p.b[p.i] == ',' {
			p.i++
			continue
		}
		if p.b[p.i] == ']' {
			p.i++
			return out, nil
		}
		return nil, p.fail("expected ',' or ']'")
	}
}

func (p *parser) number() (any, error) {
	start := p.i
	if p.b[p.i] == '-' {
		p.i++
	}
	switch {
	case p.i < len(p.b) && p.b[p.i] == '0':
		p.i++
	case p.i < len(p.b) && p.b[p.i] >= '1' && p.b[p.i] <= '9':
		for p.i < len(p.b) && p.b[p.i] >= '0' && p.b[p.i] <= '9' {
			p.i++
		}
	default:
		return nil, p.fail("invalid number")
	}
	if p.i < len(p.b) && p.b[p.i] == '.' {
		p.i++
		if p.i >= len(p.b) || p.b[p.i] < '0' || p.b[p.i] > '9' {
			return nil, p.fail("invalid number fraction")
		}
		for p.i < len(p.b) && p.b[p.i] >= '0' && p.b[p.i] <= '9' {
			p.i++
		}
	}
	if p.i < len(p.b) && (p.b[p.i] == 'e' || p.b[p.i] == 'E') {
		p.i++
		if p.i < len(p.b) && (p.b[p.i] == '+' || p.b[p.i] == '-') {
			p.i++
		}
		if p.i >= len(p.b) || p.b[p.i] < '0' || p.b[p.i] > '9' {
			return nil, p.fail("invalid number exponent")
		}
		for p.i < len(p.b) && p.b[p.i] >= '0' && p.b[p.i] <= '9' {
			p.i++
		}
	}
	if p.i-start > MaxNumberLength {
		return nil, &ParseError{start, "number longer than MaxNumberLength"}
	}
	return Number(p.b[start:p.i]), nil
}

func (p *parser) str() (string, error) {
	p.i++ // opening quote
	var sb strings.Builder
	for {
		if p.i >= len(p.b) {
			return "", p.fail("unterminated string")
		}
		c := p.b[p.i]
		switch {
		case c == '"':
			p.i++
			return sb.String(), nil
		case c < 0x20:
			return "", p.fail("control character in string")
		case c == '\\':
			if p.i+1 >= len(p.b) {
				return "", p.fail("unterminated escape")
			}
			e := p.b[p.i+1]
			p.i += 2
			switch e {
			case '"', '\\', '/':
				sb.WriteByte(e)
			case 'b':
				sb.WriteByte('\b')
			case 'f':
				sb.WriteByte('\f')
			case 'n':
				sb.WriteByte('\n')
			case 'r':
				sb.WriteByte('\r')
			case 't':
				sb.WriteByte('\t')
			case 'u':
				r, err := p.hex4()
				if err != nil {
					return "", err
				}
				if utf16.IsSurrogate(r) {
					// Only a high surrogate followed by a low one is a character.
					if r >= 0xDC00 || p.i+1 >= len(p.b) || p.b[p.i] != '\\' || p.b[p.i+1] != 'u' {
						return "", p.fail("unpaired surrogate")
					}
					p.i += 2
					low, err := p.hex4()
					if err != nil {
						return "", err
					}
					if low < 0xDC00 || low > 0xDFFF {
						return "", p.fail("unpaired surrogate")
					}
					r = utf16.DecodeRune(r, low)
				}
				sb.WriteRune(r)
			default:
				return "", p.fail("invalid escape")
			}
		default:
			_, size := utf8.DecodeRune(p.b[p.i:])
			sb.Write(p.b[p.i : p.i+size])
			p.i += size
		}
	}
}

func (p *parser) hex4() (rune, error) {
	if p.i+4 > len(p.b) {
		return 0, p.fail("short unicode escape")
	}
	v, err := strconv.ParseUint(string(p.b[p.i:p.i+4]), 16, 32)
	if err != nil {
		return 0, p.fail("invalid unicode escape")
	}
	p.i += 4
	return rune(v), nil
}

// Canonical writes v in the canonical form both languages share: object keys
// sorted by code point, no insignificant space, integers in plain decimal
// form, and strings escaped only where JSON requires it (quote, backslash and
// control characters; \b \t \n \f \r as short escapes, others as \u00xx).
// Non-integer numbers have no canonical form here and are refused. v must be
// a value as Parse returns it; anything else (other Go types, invalid UTF-8,
// a number that is not exact JSON integer text, more than 64 levels) is
// refused rather than guessed.
func Canonical(v any) ([]byte, error) {
	var b bytes.Buffer
	if err := canonical(&b, v, 0); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func canonical(b *bytes.Buffer, v any, depth int) error {
	if depth > maxDepth {
		return fmt.Errorf("value nested deeper than 64 levels")
	}
	switch x := v.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		if x {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case Number:
		n, ok := x.Integer()
		if !ok {
			return fmt.Errorf("non-integer number %s has no canonical form", x)
		}
		b.WriteString(n.String())
	case string:
		if !utf8.ValidString(x) {
			return fmt.Errorf("string is not valid UTF-8")
		}
		canonicalString(b, x)
	case []any:
		b.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := canonical(b, e, depth+1); err != nil {
				return err
			}
		}
		b.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			if !utf8.ValidString(k) {
				return fmt.Errorf("object key is not valid UTF-8")
			}
			canonicalString(b, k)
			b.WriteByte(':')
			if err := canonical(b, x[k], depth+1); err != nil {
				return err
			}
		}
		b.WriteByte('}')
	default:
		return fmt.Errorf("unsupported value %T", v)
	}
	return nil
}

func canonicalString(b *bytes.Buffer, s string) {
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\t':
			b.WriteString(`\t`)
		case '\n':
			b.WriteString(`\n`)
		case '\f':
			b.WriteString(`\f`)
		case '\r':
			b.WriteString(`\r`)
		default:
			if r < 0x20 {
				fmt.Fprintf(b, `\u%04x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
}
