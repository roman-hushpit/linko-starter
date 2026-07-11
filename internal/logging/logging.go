package logging

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"
)

type contextKey string

const LogContextKey contextKey = "log_context"

type LogContext struct {
	Username string
}

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
			logCtx := &LogContext{}
			ctx := context.WithValue(r.Context(), LogContextKey, logCtx)
			r = r.WithContext(ctx)

			spyRead := &spyReadCloser{
				ReadCloser: r.Body,
			}
			r.Body = spyRead
			spyRW := &spyResponseWriter{ResponseWriter: w}
			start := time.Now()
			next.ServeHTTP(spyRW, r)

			var attributes []slog.Attr
			attributes = append(attributes, slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("client_ip", r.RemoteAddr),
				slog.Duration("duration", time.Since(start)),
				slog.Int("response_body_bytes", spyRW.bytesWritten),
				slog.Int("response_status", spyRW.statusCode),
				slog.Int("request_body_bytes", spyRead.bytesRead),
			)
			if logCtx.Username != "" {
				attributes = append(attributes, slog.String("user", logCtx.Username))
			}

			logger.LogAttrs(r.Context(), slog.LevelInfo, "Served request", attributes...)
		})
	}
}
