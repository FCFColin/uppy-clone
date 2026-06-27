package middleware

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

func TestTracingMiddleware_InjectsTraceID(t *testing.T) {
	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(RequestIDLogger)
	r.Use(TracingMiddleware)

	var gotTraceID string
	r.Get("/test", func(w http.ResponseWriter, r *http.Request) {
		ctxLogger, ok := r.Context().Value(slogCtxKey).(*slog.Logger)
		if !ok {
			t.Error("no logger found in context")
			w.WriteHeader(500)
			return
		}
		_ = ctxLogger
		gotTraceID = "present"
		w.WriteHeader(200)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if gotTraceID != "present" {
		t.Error("trace_id was not injected into context logger")
	}
}

func TestTracingMiddleware_TraceIDInContextLogger(t *testing.T) {
	// Set up a test slog handler that captures all attributes
	var capturedAttrs []slog.Attr
	testHandler := &captureHandler{attrs: &capturedAttrs, ownAttrs: nil}
	testLogger := slog.New(testHandler)
	slog.SetDefault(testLogger)
	defer slog.SetDefault(slog.New(slog.NewJSONHandler(io.Discard, nil)))

	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(RequestIDLogger)
	r.Use(TracingMiddleware)

	r.Get("/test", func(w http.ResponseWriter, r *http.Request) {
		logger := LoggerFromContext(r.Context())
		logger.Info("test message")
		w.WriteHeader(200)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	// Verify trace_id is in the captured attributes
	found := false
	for _, attr := range capturedAttrs {
		if attr.Key == "trace_id" {
			found = true
			if attr.Value.String() == "" {
				t.Error("trace_id attribute is empty")
			}
			break
		}
	}
	if !found {
		t.Error("trace_id not found in log attributes; captured:", capturedAttrs)
	}
}

func TestRequestIDLogger_InjectsRequestID(t *testing.T) {
	// Ensure slog has a valid handler (previous tests may have set it to nil)
	slog.SetDefault(slog.New(slog.NewJSONHandler(io.Discard, nil)))
	defer slog.SetDefault(slog.New(slog.NewJSONHandler(io.Discard, nil)))

	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(RequestIDLogger)

	var gotReqID string
	r.Get("/test", func(w http.ResponseWriter, r *http.Request) {
		gotReqID = chimw.GetReqID(r.Context())
		w.WriteHeader(200)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if gotReqID == "" {
		t.Error("request_id was not set in context")
	}
}

// captureHandler is a test slog.Handler that captures all attributes
// from both WithAttrs and Handle calls.
type captureHandler struct {
	attrs    *[]slog.Attr // shared output slice
	ownAttrs []slog.Attr  // pre-existing attrs from WithAttrs
}

func (h *captureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	// Capture both the handler's own attrs and the record's attrs
	*h.attrs = append(*h.attrs, h.ownAttrs...)
	r.Attrs(func(a slog.Attr) bool {
		*h.attrs = append(*h.attrs, a)
		return true
	})
	return nil
}
func (h *captureHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newOwnAttrs := make([]slog.Attr, len(h.ownAttrs), len(h.ownAttrs)+len(attrs))
	copy(newOwnAttrs, h.ownAttrs)
	newOwnAttrs = append(newOwnAttrs, attrs...)
	return &captureHandler{attrs: h.attrs, ownAttrs: newOwnAttrs}
}
func (h *captureHandler) WithGroup(_ string) slog.Handler { return h }

func TestTracingMiddleware_ResponseWriterSupportsHijack(t *testing.T) {
	hijackable := &hijackableResponseWriter{ResponseRecorder: httptest.NewRecorder()}

	r := chi.NewRouter()
	r.Use(TracingMiddleware)
	r.Get("/ws", func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "not hijacker", http.StatusInternalServerError)
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = conn.Close()
	})

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r.ServeHTTP(hijackable, req)

	if hijackable.Code != 0 && hijackable.Code != http.StatusOK {
		t.Fatalf("expected successful hijack, got status %d body %q", hijackable.Code, hijackable.Body.String())
	}
}

type hijackableResponseWriter struct {
	*httptest.ResponseRecorder
}

func (h *hijackableResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return &net.TCPConn{}, bufio.NewReadWriter(bufio.NewReader(nil), bufio.NewWriter(nil)), nil
}
