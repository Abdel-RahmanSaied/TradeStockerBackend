package telemetry

import (
	"log/slog"
	"net/http"
	"regexp"
	"time"
)

var requestID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

// HTTP continues the caller's W3C trace (or starts one), stores it in the
// request context and returns it in the traceparent and X-Trace-Id response
// headers. A valid X-Request-Id is echoed and logged. One event is logged per
// request: method, path (never the query), status, size and duration. Headers
// and bodies are never logged.
func HTTP(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, t := Continue(r.Context(), r.Header.Get("traceparent"))
		w.Header().Set("traceparent", t.Traceparent())
		w.Header().Set("X-Trace-Id", t.TraceID)
		attrs := []any{"method", r.Method, "path", r.URL.Path}
		if id := r.Header.Get("X-Request-Id"); requestID.MatchString(id) {
			w.Header().Set("X-Request-Id", id)
			attrs = append(attrs, "request_id", id)
		}
		sw := &statusWriter{ResponseWriter: w}
		start := time.Now()
		next.ServeHTTP(sw, r.WithContext(ctx))
		if sw.status == 0 {
			sw.status = http.StatusOK
		}
		attrs = append(attrs, "status", sw.status, "bytes", sw.bytes, "duration_ms", time.Since(start).Milliseconds())
		logger.InfoContext(ctx, "http request", attrs...)
	})
}
