package middleware

import (
	"net/http"
)

type flushWriter struct {
	http.ResponseWriter
	flushed bool
}

func (f *flushWriter) Flush() {
	f.flushed = true
}
