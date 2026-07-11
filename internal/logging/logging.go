package logging

import (
	"io"
	"log/slog"
	"net/http"
	"time"
)

type spyReadCloser struct {
	io.ReadCloser
	bytesRead int
}

func (r *spyReadCloser) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	r.bytesRead += n
	return n, err
}

type spyResponseWriter struct {
	http.ResponseWriter
	bytesWritten int
	statusCode   int
}

func (w *spyResponseWriter) Write(p []byte) (int, error) {
	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(p)
	w.bytesWritten += n
	return n, err
}

func (w *spyResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func RequestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			spyRead := &spyReadCloser{
				ReadCloser: r.Body,
			}
			r.Body = spyRead
			spyRW := &spyResponseWriter{ResponseWriter: w}
			start := time.Now()
			next.ServeHTTP(spyRW, r)

			logger.Info("Served request",
				slog.Duration("duration", time.Since(start)),
				slog.Int("response_body_bytes", spyRW.bytesWritten),
				slog.Int("response_status", spyRW.statusCode),
				slog.Int("request_body_bytes", spyRead.bytesRead),
			)
		})
	}
}
