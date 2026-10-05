package main

import (
	"context"
	"errors"
	"github.com/kuny/glypha/internal/compiler"
	"github.com/kuny/glypha/internal/content"
	"github.com/kuny/glypha/internal/store"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/kuny/glypha/internal/httpserver"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		client := &http.Client{Timeout: 2 * time.Second}
		response, err := client.Get("http://127.0.0.1:8080/healthz")
		if err != nil {
			os.Exit(1)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		return
	}
	if err := run(); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	address := os.Getenv("GLYPHA_ADDR")
	if address == "" {
		address = "127.0.0.1:8080"
	}
	assets := os.Getenv("GLYPHA_WEB_DIR")
	if assets == "" {
		assets = "renderer/dist"
	}

	fontPath := os.Getenv("GLYPHA_FONT_PATH")
	if fontPath == "" {
		fontPath = "renderer/public/fonts/noto-sans-jp/NotoSansJP.ttf"
	}
	compiler, err := compiler.New(fontPath, content.Canvas{Width: 1920, Height: 1080})
	if err != nil {
		return err
	}
	database := os.Getenv("GLYPHA_DB_PATH")
	if database == "" {
		database = "data/glypha.db"
	}
	if err := os.MkdirAll(filepath.Dir(database), 0700); err != nil {
		return err
	}
	state, err := store.Open(database, compiler, store.SystemClock{})
	if err != nil {
		return err
	}
	defer state.Close()
	server := &http.Server{
		Addr: address, Handler: httpserver.New(os.DirFS(assets), httpserver.NewAPI(compiler, state)),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second,
		WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()
	slog.Info("server starting", "address", address, "web_directory", assets)
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			return err
		}
		err := <-done
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
