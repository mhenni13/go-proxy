package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"
	"github.com/mhenni13/go-proxy/internal/logging"
	"github.com/mhenni13/go-proxy/internal/proxy"
	"github.com/mhenni13/go-proxy/internal/config"


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
