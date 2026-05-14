package api

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

type loggingResponseWriter struct {
	http.ResponseWriter
	status int
}

func (lrw *loggingResponseWriter) Write(b []byte) (int, error) {
	if lrw.status == 0 {
		lrw.status = http.StatusOK
	}
	return lrw.ResponseWriter.Write(b)
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.status = code
	lrw.ResponseWriter.WriteHeader(code)
}

func (lrw *loggingResponseWriter) Flush() {
	if flusher, ok := lrw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (lrw *loggingResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := lrw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("wrapped response writer does not support hijacking")
	}
	return hijacker.Hijack()
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lrw := &loggingResponseWriter{
			ResponseWriter: w,
			status:         http.StatusOK,
		}

		next.ServeHTTP(lrw, r)

		// #nosec G706 -- user-controlled fields are sanitized via sanitizeLogField before logging.
		log.Printf(
			"http_request method=%s path=%s status=%d duration_ms=%d remote=%s",
			sanitizeLogField(r.Method),
			sanitizeLogField(r.URL.RequestURI()),
			lrw.status,
			time.Since(start).Milliseconds(),
			sanitizeLogField(r.RemoteAddr),
		)
	})
}

func sanitizeLogField(v string) string {
	cleaned := strings.ReplaceAll(v, "\n", "_")
	cleaned = strings.ReplaceAll(cleaned, "\r", "_")
	return cleaned
}
