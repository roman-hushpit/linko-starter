package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"boot.dev/linko/internal/linkoerr"
	"boot.dev/linko/internal/store"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	httpPort := flag.Int("port", 8899, "port to listen on")
	dataDir := flag.String("data", "./data", "directory to store data")
	flag.Parse()

	status := run(ctx, cancel, *httpPort, *dataDir)
	cancel()
	os.Exit(status)
}

func run(ctx context.Context, cancel context.CancelFunc, httpPort int, dataDir string) int {
	standardLogger, closeFunc, err := initializeLogger(os.Getenv("LINKO_LOG_FILE"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize logger: %v\n", err)
		return 1
	}
	st, err := store.New(dataDir, standardLogger)
	if err != nil {
		standardLogger.Error(fmt.Sprintf("failed to create store: %v\n", err))
		return 1
	}
	s := newServer(*st, httpPort, cancel, standardLogger)
	var serverErr error
	go func() {
		serverErr = s.start()
	}()

	<-ctx.Done()
	defer func() {
		err := closeFunc()
		if err != nil {
			fmt.Printf("Failed to flush buffer: %v\n", err)
			return
		}
	}()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.shutdown(shutdownCtx); err != nil {
		standardLogger.Error(fmt.Sprintf("failed to shutdown server: %v\n", err))
		return 1
	}
	if serverErr != nil {
		standardLogger.Error(fmt.Sprintf("server error: %v\n", serverErr))
		return 1
	}
	return 0
}

func initializeLogger(logFile string) (*slog.Logger, closeFunc, error) {
	debugHandler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level:       slog.LevelDebug,
		ReplaceAttr: ReplaceAttr,
	})

	if logFile != "" {
		file, err := os.OpenFile(logFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to open log file: %w", err)
		}
		bufferedFile := bufio.NewWriterSize(file, 8192)
		multiWriter := io.MultiWriter(os.Stderr, bufferedFile)
		closer := func() error {
			if err := bufferedFile.Flush(); err != nil {
				fmt.Printf("failed to flush buffered file: %v\n", err)
				return err
			}
			return nil
		}
		infoHandler := slog.NewJSONHandler(multiWriter, &slog.HandlerOptions{
			Level:       slog.LevelInfo,
			ReplaceAttr: ReplaceAttr,
		})
		logger := slog.New(slog.NewMultiHandler(
			debugHandler,
			infoHandler,
		))
		return logger, closer, nil
	}
	return slog.New(debugHandler), func() error {
		return nil
	}, nil
}

func ReplaceAttr(groups []string, a slog.Attr) slog.Attr {
	if a.Key == "error" {
		err, ok := a.Value.Any().(error)
		if !ok {
			return a
		}
		attributes := []slog.Attr{
			slog.String("message", err.Error()),
		}
		attributes = append(attributes, linkoerr.Attrs(err)...)
		return slog.GroupAttrs("error", attributes...)
	}
	return a
}

type closeFunc func() error
