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
	configPath := flag.String("c", "internal/config/config.yaml", "Path to config file")
	configPathLong := flag.String("config", "internal/config/config.yaml", "Path to config file")
	wafLogPath := flag.String("l", "", "Path to WAF log file (overrides config)")
	wafLogPathLong := flag.String("log", "", "Path to WAF log file (overrides config)")
	ipListPath := flag.String("s", "", "Path to IP list file (allowlist/blocklist, overrides config)")
	ipListPathLong := flag.String("security-list", "", "Path to IP list file (allowlist/blocklist, overrides config)")
	flag.Parse()

	// Use long flag if provided, otherwise short flag
	finalConfigPath := *configPath
	if *configPathLong != "internal/config/config.yaml" {
		finalConfigPath = *configPathLong
	}

	finalWafLog := *wafLogPath
	if *wafLogPathLong != "" {
		finalWafLog = *wafLogPathLong
	}

	finalIPList := *ipListPath
	if *ipListPathLong != "" {
		finalIPList = *ipListPathLong
	}

	cfg, err := config.LoadConfig(finalConfigPath)
	if err != nil {
		log.Fatalf("❌ Failed to load config: %v", err)
	}

	// Override config with CLI flags
	if finalWafLog != "" {
		for i := range cfg.APIs {
			if cfg.APIs[i].Security.WAF.Enabled {
				cfg.APIs[i].Security.WAF.LogFile = finalWafLog
			}
		}
	}

	if finalIPList != "" {
		for i := range cfg.APIs {
			if cfg.APIs[i].Security.IPBlocker.Enabled {
				if cfg.APIs[i].Security.IPBlocker.Mode == "allowlist" {
					cfg.APIs[i].Security.IPBlocker.AllowlistFile = finalIPList
				} else if cfg.APIs[i].Security.IPBlocker.Mode == "blocklist" {
					cfg.APIs[i].Security.IPBlocker.BlocklistFile = finalIPList
				}
			}
		}
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

	if cfg.Config.TLS {
		log.Printf("🚀 Proxy starting with TLS on :%d ...", cfg.Config.Port)
		if err := server.ListenAndServeTLS(cfg.Config.TLSCertFile, cfg.Config.TLSKeyFile); err != nil {
			log.Fatalf("❌ Proxy server error: %v", err)
		}
	} else {
		log.Printf("🚀 Proxy starting on :%d ...", cfg.Config.Port)
		if err := server.ListenAndServe(); err != nil {
			log.Fatalf("❌ Proxy server error: %v", err)
		}
	}
}
