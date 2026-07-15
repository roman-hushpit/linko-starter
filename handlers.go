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
	w.Header().Set("Content-Type", "text/html")
	io.WriteString(w, indexPage)
}

func (s *server) handlerLogin(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (s *server) handlerShortenLink(w http.ResponseWriter, r *http.Request) {
	user, ok := r.Context().Value(UserContextKey).(string)
	if !ok || user == "" {
		logging.HttpError(w, pkgerr.WithStack(errors.New("unauthorized")), http.StatusUnauthorized, r.Context())
		return
	}
	longURL := r.FormValue("url")
	if longURL == "" {
		logging.HttpError(w, pkgerr.WithStack(errors.New("missing url parameter")), http.StatusBadRequest, r.Context())
		return
	}
	u, err := url.Parse(longURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		logging.HttpError(w, pkgerr.WithStack(errors.New("invalid URL: must include scheme (http/https) and host")),
			http.StatusBadRequest, r.Context())
		return
	}
	if err := checkDestination(longURL); err != nil {
		logging.HttpError(w, pkgerr.WithStack(errors.New(fmt.Sprintf("invalid target URL: %v", err))), http.StatusBadRequest, r.Context())
		return
	}
	shortCode, err := s.store.Create(r.Context(), longURL)
	if err != nil {
		logging.HttpError(w, pkgerr.WithStack(errors.New("failed to shorten URL")), http.StatusInternalServerError, r.Context())
		return
	}
	s.logger.Info("Successfully generated short code", slog.String("shortCode", shortCode), slog.String("long_url", obfuscateUrl(longURL)))
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusCreated)
	io.WriteString(w, shortCode)
}

func (s *server) handlerRedirect(w http.ResponseWriter, r *http.Request) {
	longURL, err := s.store.Lookup(r.Context(), r.PathValue("shortCode"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			logging.HttpError(w, pkgerr.WithStack(errors.New("not found")), http.StatusNotFound, r.Context())
		} else {
			s.logger.Error("failed to lookup URL", "error", err)
			logging.HttpError(w, pkgerr.WithStack(errors.New("internal server error")), http.StatusInternalServerError, r.Context())
		}
		return
	}
	if err := checkDestination(longURL); err != nil {
		logging.HttpError(w, pkgerr.WithStack(errors.New("destination unavailable")), http.StatusBadGateway, r.Context())
		return
	}

	redirectsMu.Lock()
	redirects = append(redirects, longURL)
	redirectsMu.Unlock()

	http.Redirect(w, r, longURL, http.StatusFound)
}

func (s *server) handlerListURLs(w http.ResponseWriter, r *http.Request) {
	codes, err := s.store.List(r.Context())
	if err != nil {
		s.logger.Error("failed to list URLs", "error", err)
		logging.HttpError(w, pkgerr.WithStack(errors.New("failed to list URLs")), http.StatusInternalServerError, r.Context())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(codes)
}

func (s *server) handlerStats(w http.ResponseWriter, _ *http.Request) {
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
