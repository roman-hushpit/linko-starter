package logging

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	"boot.dev/linko/internal/linkoerr"
)

type contextKey string

const LogContextKey contextKey = "log_context"

type LogContext struct {
	Username string
	Error    error
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
			xRequestID := r.Header.Get("X-Request-Id")
			if xRequestID == "" {
				xRequestID = rand.Text()
			}
			logCtx := &LogContext{}
			ctx := context.WithValue(r.Context(), LogContextKey, logCtx)
			r = r.WithContext(ctx)

			spyRead := &spyReadCloser{
				ReadCloser: r.Body,
			}
			r.Body = spyRead
			spyRW := &spyResponseWriter{ResponseWriter: w}
			start := time.Now()
			spyRW.Header().Set("X-Request-Id", xRequestID)
			next.ServeHTTP(spyRW, r)

			var attributes []slog.Attr
			attributes = append(attributes, slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("client_ip", redactIP(r.RemoteAddr)),
				slog.Duration("duration", time.Since(start)),
				slog.Int("response_body_bytes", spyRW.bytesWritten),
				slog.Int("response_status", spyRW.statusCode),
				slog.Int("request_body_bytes", spyRead.bytesRead),
				slog.String("request_id", xRequestID),
			)
			if logCtx.Username != "" {
				attributes = append(attributes, slog.String("user", logCtx.Username))
			}
			if logCtx.Username != "" {
				attributes = append(attributes, slog.String("user", logCtx.Username))
			}
			if logCtx.Error != nil {
				attributes = append(attributes, slog.GroupAttrs("error", linkoerr.ErrorAttrs(logCtx.Error)...))
			}
			logger.LogAttrs(r.Context(), slog.LevelInfo, "Served request", attributes...)
		})
	}
}

func HttpError(w http.ResponseWriter, err error, status int, ctx context.Context) {
	if logCtx, ok := ctx.Value(LogContextKey).(*LogContext); ok {
		logCtx.Error = err
	}
	generalizedError := err
	switch status {
	case 401, 403, 500:
		generalizedError = errors.New(http.StatusText(status))
	}
	http.Error(w, generalizedError.Error(), status)
}

func redactIP(client_ip string) string {
	host, _, err := net.SplitHostPort(client_ip)
	if err != nil {
		return client_ip
	}
	ipAddress := net.ParseIP(host)
	if ipAddress == nil {
		return client_ip
	}
	to4Ip := ipAddress.To4()
	if to4Ip == nil {
		return client_ip
	}
	return fmt.Sprintf("%d.%d.%d.x", to4Ip[0], to4Ip[1], to4Ip[2])
}
