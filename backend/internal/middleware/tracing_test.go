package middleware

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

type flushWriter struct {
	http.ResponseWriter
	flushed bool
}

func (f *flushWriter) Flush() {
	f.flushed = true
}

func TestResponseWriter_Flush(t *testing.T) {
	base := &flushWriter{ResponseWriter: httptest.NewRecorder()}
	rw := &responseWriter{ResponseWriter: base, statusCode: 200}
	rw.Flush()
	if !base.flushed {
		t.Fatal("Flush should delegate to underlying Flusher")
	}
}

func TestResponseWriter_Flush_NoFlusher(t *testing.T) {
	rw := &responseWriter{ResponseWriter: httptest.NewRecorder(), statusCode: 200}
	rw.Flush() // must not panic when underlying writer is not a Flusher
}

func TestResponseWriter_Hijack_NotSupported(t *testing.T) {
	rw := &responseWriter{ResponseWriter: httptest.NewRecorder(), statusCode: 200}
	conn, rwBuf, err := rw.Hijack()
	if err == nil || conn != nil || rwBuf != nil {
		t.Fatalf("Hijack = conn=%v rw=%v err=%v", conn, rwBuf, err)
	}
}

type hijackRecorder struct {
	*httptest.ResponseRecorder
}

func (h *hijackRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, nil
}

func TestResponseWriter_Hijack_Delegates(t *testing.T) {
	base := &hijackRecorder{ResponseRecorder: httptest.NewRecorder()}
	rw := &responseWriter{ResponseWriter: base, statusCode: 200}
	_, _, err := rw.Hijack()
	if err != nil {
		t.Fatalf("Hijack: %v", err)
	}
}
