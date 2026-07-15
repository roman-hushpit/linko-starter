package main

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"boot.dev/linko/internal/tracing"
)

func checkDestination(targetURL string, ctx context.Context) error {
	_, span := tracing.Tracer.Start(ctx, "http.verify_destination")
	defer span.End()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("destination unreachable: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("destination returned status %d", resp.StatusCode)
	}
	return nil
}
