package main

import (
	"log"
	"os"

	"github.com/sspaeti/minimal-newsletter-survey/internal/server"
	"github.com/sspaeti/minimal-newsletter-survey/internal/store"
)

func main() {
	cfg := server.Config{
		DBPath:     getenv("SURVEY_DB_PATH", "/var/db/survey/votes.duckdb"),
		HTTPAddr:   getenv("SURVEY_HTTP_ADDR", "127.0.0.1:8080"),
		QuackAddr:  getenv("SURVEY_QUACK_ADDR", "127.0.0.1:9494"),
		QuackToken: os.Getenv("SURVEY_QUACK_TOKEN"),
		AdminToken: os.Getenv("SURVEY_ADMIN_TOKEN"),
		SiteURL:    getenv("SURVEY_SITE_URL", "https://pollmd.ssp.sh"),
	}
	// Quack is optional — if no token is set, the admin HTTP endpoints
	// (which don't need Quack) can still operate on the local DB.
	if cfg.QuackToken == "" {
		log.Println("SURVEY_QUACK_TOKEN not set — Quack admin disabled")
		cfg.QuackAddr = "" // prevent store from attempting quack_serve
	}

	st, err := store.Open(cfg.DBPath, cfg.QuackAddr, cfg.QuackToken)
	if err != nil {
		log.Fatalf("store open: %v", err)
	}
	defer st.Close()

	srv := server.New(cfg, st)
	if cfg.QuackAddr != "" {
		log.Printf("survey: HTTP on %s, Quack on %s", cfg.HTTPAddr, cfg.QuackAddr)
	} else {
		log.Printf("survey: HTTP on %s (Quack disabled)", cfg.HTTPAddr)
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
