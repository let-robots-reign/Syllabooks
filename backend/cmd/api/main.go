package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
	"syllabooks/internal/handlers"

	"github.com/jackc/pgx/v5/pgxpool"
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

	var yandex, vk *handlers.Provider
	if clientID := os.Getenv("YANDEX_CLIENT_ID"); clientID != "" {
		clientSecret := os.Getenv("YANDEX_CLIENT_SECRET")
		if clientSecret == "" || publicURL == "" {
			return errors.New("YANDEX_CLIENT_ID is set, so YANDEX_CLIENT_SECRET and PUBLIC_URL must be too")
		}
		yandex = handlers.NewYandex(clientID, clientSecret, publicURL+"/api/auth/yandex/callback")
	} else {
		log.Print("YANDEX_CLIENT_ID is not set: Yandex login is disabled")
	}
	if clientID := os.Getenv("VK_CLIENT_ID"); clientID != "" {
		if publicURL == "" {
			return errors.New("VK_CLIENT_ID is set, so PUBLIC_URL must be too")
		}
		vk = handlers.NewVK(clientID, publicURL+"/api/auth/vk/callback")
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

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}

	srv := &handlers.Server{
		Pool:          pool,
		Yandex:        yandex,
		VK:            vk,
		SecureCookies: strings.HasPrefix(publicURL, "https://"),
		Frontend:      frontend,
	}
	log.Printf("listening on %s", addr)
	return http.ListenAndServe(addr, srv.Routes())
}
