package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"syllabooks/internal/handlers"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	// Configuration comes from environment variables: docker-compose.yaml sets them
	// in development, the service manager sets them in production.
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return errors.New("DATABASE_URL is not set")
	}
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	// The address the browser uses, e.g. https://syllabooks.ru. OAuth redirect
	// URIs are built from it.
	publicURL := strings.TrimSuffix(os.Getenv("PUBLIC_URL"), "/")
	bookCoversDir := os.Getenv("BOOK_COVERS_DIR")
	if bookCoversDir == "" {
		bookCoversDir = "data/covers"
	}
	loanDays := int32(21)
	if value := os.Getenv("LOAN_DAYS"); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 32)
		if err != nil || parsed <= 0 {
			return errors.New("LOAN_DAYS must be a positive integer")
		}
		loanDays = int32(parsed)
	}
	shelfCode := strings.TrimSpace(os.Getenv("SHELF_CODE"))
	if shelfCode == "" {
		return errors.New("SHELF_CODE is not set")
	}

	var yandex, vk *handlers.Provider
	if clientID := os.Getenv("YANDEX_CLIENT_ID"); clientID != "" {
		clientSecret := os.Getenv("YANDEX_CLIENT_SECRET")
		if clientSecret == "" || publicURL == "" {
			return errors.New("YANDEX_CLIENT_ID is set, so YANDEX_CLIENT_SECRET and PUBLIC_URL must be too")
		}
		yandex = handlers.NewYandex(clientID, clientSecret, publicURL+"/api/auth/callback/yandex")
	} else {
		log.Print("YANDEX_CLIENT_ID is not set: Yandex login is disabled")
	}
	if clientID := os.Getenv("VK_CLIENT_ID"); clientID != "" {
		if publicURL == "" {
			return errors.New("VK_CLIENT_ID is set, so PUBLIC_URL must be too")
		}
		vk = handlers.NewVK(clientID, publicURL+"/api/auth/callback/vk")
	} else {
		log.Print("VK_CLIENT_ID is not set: VK login is disabled")
	}

	frontend, err := fs.Sub(dist, "dist")
	if err != nil {
		return fmt.Errorf("open embedded frontend: %w", err)
	}
	if _, err := fs.Stat(frontend, "index.html"); err != nil {
		log.Print("no frontend built into this binary (see make build): serving the API only")
	}

	startupCtx, cancelStartup := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelStartup()
	pool, err := pgxpool.New(startupCtx, databaseURL)
	if err != nil {
		return fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(startupCtx); err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}

	srv := &handlers.Server{
		Pool:               pool,
		LoanDays:           loanDays,
		ShelfCode:          shelfCode,
		Yandex:             yandex,
		VK:                 vk,
		SecureCookies:      strings.HasPrefix(publicURL, "https://"),
		Frontend:           frontend,
		BookCoversDir:      bookCoversDir,
		HTTPClient:         &http.Client{Timeout: 10 * time.Second},
		OpenLibraryBaseURL: "https://openlibrary.org",
		GoogleBooksBaseURL: "https://www.googleapis.com/books/v1",
		GoogleBooksAPIKey:  os.Getenv("GOOGLE_BOOKS_API_KEY"),
	}
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	shutdownSignal, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- httpServer.ListenAndServe()
	}()

	log.Printf("listening on %s", addr)
	select {
	case err := <-serverErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTP: %w", err)
	case <-shutdownSignal.Done():
		stop()
		log.Print("shutting down")
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shut down HTTP server: %w", err)
	}
	if err := <-serverErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve HTTP: %w", err)
	}
	return nil
}
