package main

import (
	"github.com/mhenni13/go-proxy"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Path to config file")
	flag.Parse()

	cfg, err := go_proxy.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("❌ Failed to load config: %v", err)
	}

	mux := http.NewServeMux()
	go_proxy.RegisterAPIs(mux, cfg)

	handler := go_proxy.LoggingMiddleware(mux)

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Config.Port),
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("🚀 Proxy starting on :%d ...", cfg.Config.Port)
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("❌ Proxy server error: %v", err)
	}
}
