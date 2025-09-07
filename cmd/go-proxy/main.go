package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/mhenni13/go-proxy/internal/config"
	"github.com/mhenni13/go-proxy/internal/logging"
	"github.com/mhenni13/go-proxy/internal/proxy"
)

func main() {
	configPath := flag.String("config", "internal/config/config.yaml", "Path to config file")
	flag.Parse()

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("❌ Failed to load config: %v", err)
	}

	mux := http.NewServeMux()
	proxy.RegisterAPIs(mux, cfg)

	handler := logging.LoggingMiddleware(mux)

	readTimeout, err := time.ParseDuration(cfg.Config.ReadTimeout)
	if err != nil {
		log.Fatalf("Invalid read_timeout: %v", err)
	}

	writeTimeout, err := time.ParseDuration(cfg.Config.WriteTimeout)
	if err != nil {
		log.Fatalf("Invalid write_timeout: %v", err)
	}

	idleTimeout, err := time.ParseDuration(cfg.Config.IdleTimeout)
	if err != nil {
		log.Fatalf("Invalid idle_timeout: %v", err)
	}

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Config.Port),
		Handler:      handler,
		ReadTimeout:  readTimeout * time.Second,
		WriteTimeout: writeTimeout * time.Second,
		IdleTimeout:  idleTimeout * time.Second,
	}

	log.Printf("🚀 Proxy starting on :%d ...", cfg.Config.Port)
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("❌ Proxy server error: %v", err)
	}
}
