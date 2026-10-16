package observability

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/smartreversal/smartreversal/apps/api/internal/datashield"
)

// TraceHeader carries the trace identifier on requests and responses.
const TraceHeader = "X-Trace-Id"

type traceIDKey struct{}

// validTraceID limits what a caller can place in logs and response headers.
var validTraceID = regexp.MustCompile(`^[A-Za-z0-9._-]{8,64}$`)

// NewLogger returns a structured logger. Development uses readable text,
// every other environment uses JSON. Values that look like account or card
// numbers are masked before they are written, as a safety net: code should log
// tokens, not numbers.
func NewLogger(env, level string) *slog.Logger {
	return NewLoggerTo(os.Stdout, env, level)
}

// NewLoggerTo is NewLogger writing to w.
func NewLoggerTo(w io.Writer, env, level string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: parseLevel(level), ReplaceAttr: redactAttr}
	if env == "development" {
		return slog.New(slog.NewTextHandler(w, opts))
	}
	return slog.New(slog.NewJSONHandler(w, opts))
}

// redactAttr masks number-like digit runs in strings, messages and errors.
func redactAttr(_ []string, a slog.Attr) slog.Attr {
	switch a.Value.Kind() {
	case slog.KindString:
		a.Value = slog.StringValue(datashield.Redact(a.Value.String()))
	case slog.KindAny:
		if err, ok := a.Value.Any().(error); ok {
			a.Value = slog.StringValue(datashield.Redact(err.Error()))
		}
	}
	return a
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

var w3cTraceparentRegex = regexp.MustCompile(`^00-([0-9a-f]{32})-([0-9a-f]{16})-([0-9a-f]{2})$`)

func formatTraceparent(traceID string) string {
	hexOnly := strings.ToLower(strings.ReplaceAll(traceID, "-", ""))
	for len(hexOnly) < 32 {
		hexOnly += "0"
	}
	if len(hexOnly) > 32 {
		hexOnly = hexOnly[:32]
	}
	return "00-" + hexOnly + "-0000000000000001-01"
}

// TraceMiddleware reads traceparent or X-Trace-Id when safe, otherwise generates one,
// stores it in the request context and echoes both headers on the response.
func TraceMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(TraceHeader)
		tp := r.Header.Get("traceparent")
		if id == "" && tp != "" {
			if m := w3cTraceparentRegex.FindStringSubmatch(tp); len(m) == 4 {
				id = m[1]
			}
		}
		if !validTraceID.MatchString(id) {
			id = uuid.NewString()
		}
		w.Header().Set(TraceHeader, id)
		w.Header().Set("traceparent", formatTraceparent(id))
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), traceIDKey{}, id)))
	})
}

// TraceID returns the trace identifier stored in ctx, or an empty string.
func TraceID(ctx context.Context) string {
	id, _ := ctx.Value(traceIDKey{}).(string)
	return id
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// RequestLogger logs one structured line per request. It records the path but
// never the query string or headers, which may carry identifiers or tokens.
func RequestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(rec, r)

			logger.LogAttrs(r.Context(), slog.LevelInfo, "http_request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rec.status),
				slog.Int64("duration_ms", time.Since(start).Milliseconds()),
				slog.String("trace_id", TraceID(r.Context())),
			)
		})
	}
}
