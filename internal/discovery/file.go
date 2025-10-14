package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/mhenni13/go-proxy/internal/config"
	"gopkg.in/yaml.v3"
)

// FileProvider implements file-based service discovery
type FileProvider struct {
	filePath    string
	format      string // "json" or "yaml"
	upstreams   []config.Upstream
	mu          sync.RWMutex
	stopChan    chan struct{}
	watchPeriod time.Duration
}

// FileConfig holds configuration for file-based service discovery
type FileConfig struct {
	Path        string `yaml:"path"`                  // Path to file containing upstream list
	Format      string `yaml:"format,omitempty"`      // "json" or "yaml" (auto-detected if omitted)
	WatchPeriod string `yaml:"watch_period,omitempty"` // How often to check for file changes (e.g., "10s")
}

// UpstreamsFile represents the structure of the upstreams file
type UpstreamsFile struct {
	Upstreams []config.Upstream `json:"upstreams" yaml:"upstreams"`
}

// NewFileProvider creates a new file-based service discovery provider
func NewFileProvider(cfg FileConfig) (*FileProvider, error) {
	if cfg.Path == "" {
		return nil, fmt.Errorf("file path is required for file discovery")
	}

	// Auto-detect format from file extension if not specified
	format := cfg.Format
	if format == "" {
		ext := filepath.Ext(cfg.Path)
		switch ext {
		case ".json":
			format = "json"
		case ".yaml", ".yml":
			format = "yaml"
		default:
			return nil, fmt.Errorf("cannot auto-detect format from extension %s, please specify format", ext)
		}
	}

	if format != "json" && format != "yaml" {
		return nil, fmt.Errorf("unsupported file format: %s (must be json or yaml)", format)
	}

	// Parse watch period
	var watchPeriod time.Duration
	if cfg.WatchPeriod != "" {
		var err error
		watchPeriod, err = time.ParseDuration(cfg.WatchPeriod)
		if err != nil {
			return nil, fmt.Errorf("invalid watch_period: %w", err)
		}
	}

	provider := &FileProvider{
		filePath:    cfg.Path,
		format:      format,
		stopChan:    make(chan struct{}),
		watchPeriod: watchPeriod,
	}

	// Initial load
	if err := provider.loadFile(); err != nil {
		return nil, fmt.Errorf("failed to load initial file: %w", err)
	}

	// Start file watcher if watch period is configured
	if watchPeriod > 0 {
		go provider.watchFile()
	}

	log.Printf("[File] Loaded %d upstreams from %s (format: %s, watch: %v)",
		len(provider.upstreams), cfg.Path, format, watchPeriod)

	return provider, nil
}

// Discover returns upstreams from the file
func (p *FileProvider) Discover(ctx context.Context) ([]config.Upstream, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if len(p.upstreams) == 0 {
		return nil, fmt.Errorf("no upstreams loaded from file %s", p.filePath)
	}

	// Return a copy to avoid concurrent modification issues
	upstreams := make([]config.Upstream, len(p.upstreams))
	copy(upstreams, p.upstreams)

	return upstreams, nil
}

// loadFile loads upstreams from the file
func (p *FileProvider) loadFile() error {
	data, err := os.ReadFile(p.filePath)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", p.filePath, err)
	}

	var upstreamsFile UpstreamsFile

	switch p.format {
	case "json":
		if err := json.Unmarshal(data, &upstreamsFile); err != nil {
			return fmt.Errorf("failed to parse JSON from %s: %w", p.filePath, err)
		}
	case "yaml":
		if err := yaml.Unmarshal(data, &upstreamsFile); err != nil {
			return fmt.Errorf("failed to parse YAML from %s: %w", p.filePath, err)
		}
	default:
		return fmt.Errorf("unsupported format: %s", p.format)
	}

	if len(upstreamsFile.Upstreams) == 0 {
		return fmt.Errorf("no upstreams found in file %s", p.filePath)
	}

	// Apply defaults for TLS fields if not specified
	for i := range upstreamsFile.Upstreams {
		if upstreamsFile.Upstreams[i].TLS == nil {
			defaultTLS := false
			upstreamsFile.Upstreams[i].TLS = &defaultTLS
		}
		if upstreamsFile.Upstreams[i].TLSInsecure == nil {
			defaultTLSInsecure := false
			upstreamsFile.Upstreams[i].TLSInsecure = &defaultTLSInsecure
		}
	}

	p.mu.Lock()
	p.upstreams = upstreamsFile.Upstreams
	p.mu.Unlock()

	return nil
}

// watchFile periodically checks for file changes and reloads
func (p *FileProvider) watchFile() {
	ticker := time.NewTicker(p.watchPeriod)
	defer ticker.Stop()

	var lastModTime time.Time
	fileInfo, err := os.Stat(p.filePath)
	if err == nil {
		lastModTime = fileInfo.ModTime()
	}

	for {
		select {
		case <-p.stopChan:
			return
		case <-ticker.C:
			fileInfo, err := os.Stat(p.filePath)
			if err != nil {
				log.Printf("[File] Error checking file %s: %v", p.filePath, err)
				continue
			}

			// Check if file has been modified
			if fileInfo.ModTime().After(lastModTime) {
				log.Printf("[File] Detected changes in %s, reloading...", p.filePath)
				if err := p.loadFile(); err != nil {
					log.Printf("[File] Error reloading file %s: %v", p.filePath, err)
				} else {
					p.mu.RLock()
					count := len(p.upstreams)
					p.mu.RUnlock()
					log.Printf("[File] Successfully reloaded %d upstreams from %s", count, p.filePath)
				}
				lastModTime = fileInfo.ModTime()
			}
		}
	}
}

// Name returns the provider name
func (p *FileProvider) Name() string {
	return fmt.Sprintf("file:%s", filepath.Base(p.filePath))
}

// Close releases resources
func (p *FileProvider) Close() error {
	if p.watchPeriod > 0 {
		close(p.stopChan)
	}
	return nil
}
