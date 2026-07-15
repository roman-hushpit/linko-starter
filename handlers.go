package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sync"

	"boot.dev/linko/internal/logging"
	"boot.dev/linko/internal/store"
	"boot.dev/linko/internal/tracing"
	pkgerr "github.com/pkg/errors"
)

const shortURLLen = len("http://localhost:8080/") + 6

var (
	redirectsMu sync.Mutex
	redirects   []string
)

//go:embed index.html
var indexPage string

func (s *server) handlerIndex(w http.ResponseWriter, r *http.Request) {
	_, span := tracing.Tracer.Start(r.Context(), "handler.index")
	defer span.End()
	w.Header().Set("Content-Type", "text/html")
	io.WriteString(w, indexPage)
}

func (s *server) handlerLogin(w http.ResponseWriter, r *http.Request) {
	_, span := tracing.Tracer.Start(r.Context(), "handler.login")
	defer span.End()
	w.WriteHeader(http.StatusOK)
}

func (s *server) handlerShortenLink(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.Tracer.Start(r.Context(), "handler.shorten_link")
	defer span.End()
	user, ok := ctx.Value(UserContextKey).(string)
	if !ok || user == "" {
		logging.HttpError(w, pkgerr.WithStack(errors.New("unauthorized")), http.StatusUnauthorized, ctx)
		return
	}
	longURL := r.FormValue("url")
	if longURL == "" {
		logging.HttpError(w, pkgerr.WithStack(errors.New("missing url parameter")), http.StatusBadRequest, ctx)
		return
	}
	u, err := url.Parse(longURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		logging.HttpError(w, pkgerr.WithStack(errors.New("invalid URL: must include scheme (http/https) and host")),
			http.StatusBadRequest, ctx)
		return
	}
	if err := checkDestination(longURL, ctx); err != nil {
		logging.HttpError(w, pkgerr.WithStack(errors.New(fmt.Sprintf("invalid target URL: %v", err))), http.StatusBadRequest, ctx)
		return
	}
	shortCode, err := s.store.Create(ctx, longURL)
	if err != nil {
		logging.HttpError(w, pkgerr.WithStack(errors.New("failed to shorten URL")), http.StatusInternalServerError, ctx)
		return
	}
	s.logger.Info("Successfully generated short code", slog.String("shortCode", shortCode), slog.String("long_url", obfuscateUrl(longURL)))
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusCreated)
	io.WriteString(w, shortCode)
}

func (s *server) handlerRedirect(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.Tracer.Start(r.Context(), "handler.redirect")
	defer span.End()
	longURL, err := s.store.Lookup(ctx, r.PathValue("shortCode"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			logging.HttpError(w, pkgerr.WithStack(errors.New("not found")), http.StatusNotFound, ctx)
		} else {
			s.logger.Error("failed to lookup URL", "error", err)
			logging.HttpError(w, pkgerr.WithStack(errors.New("internal server error")), http.StatusInternalServerError, ctx)
		}
		return
	}
	if err := checkDestination(longURL, ctx); err != nil {
		logging.HttpError(w, pkgerr.WithStack(errors.New("destination unavailable")), http.StatusBadGateway, ctx)
		return
	}

	redirectsMu.Lock()
	redirects = append(redirects, longURL)
	redirectsMu.Unlock()

	http.Redirect(w, r, longURL, http.StatusFound)
}

func (s *server) handlerListURLs(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.Tracer.Start(r.Context(), "handler.list_urls")
	defer span.End()
	codes, err := s.store.List(r.Context())
	if err != nil {
		s.logger.Error("failed to list URLs", "error", err)
		logging.HttpError(w, pkgerr.WithStack(errors.New("failed to list URLs")), http.StatusInternalServerError, ctx)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(codes)
}

func (s *server) handlerStats(w http.ResponseWriter, r *http.Request) {
	_, span := tracing.Tracer.Start(r.Context(), "handler.stats")
	defer span.End()
	redirectsMu.Lock()
	snapshot := redirects
	redirectsMu.Unlock()

	var bytesSaved int
	for _, u := range snapshot {
		bytesSaved += len(u) - shortURLLen
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{
		"redirects":   len(snapshot),
		"bytes_saved": bytesSaved,
	})
}

func obfuscateUrl(requestUrl string) string {
	parsedUrl, err := url.Parse(requestUrl)
	if err != nil {
		return requestUrl
	}
	if _, b := parsedUrl.User.Password(); b {
		parsedUrl.User = url.UserPassword(parsedUrl.User.Username(), "REDACTED")
	}
	return parsedUrl.String()
}
