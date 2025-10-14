package discovery

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mhenni13/go-proxy/internal/config"
	"gopkg.in/yaml.v3"
)

func TestFileProvider(t *testing.T) {
	t.Run("NewFileProvider with JSON file", func(t *testing.T) {
		// Create temporary JSON file
		tmpDir := t.TempDir()
		jsonFile := filepath.Join(tmpDir, "upstreams.json")

		data := UpstreamsFile{
			Upstreams: []config.Upstream{
				{Host: "10.0.0.1", Port: 8080},
				{Host: "10.0.0.2", Port: 8080},
			},
		}

		jsonData, _ := json.Marshal(data)
		if err := os.WriteFile(jsonFile, jsonData, 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		provider, err := NewFileProvider(FileConfig{
			Path:   jsonFile,
			Format: "json",
		})

		if err != nil {
			t.Fatalf("failed to create provider: %v", err)
		}
		defer provider.Close()

		ctx := context.Background()
		upstreams, err := provider.Discover(ctx)
		if err != nil {
			t.Fatalf("discover failed: %v", err)
		}

		if len(upstreams) != 2 {
			t.Errorf("expected 2 upstreams, got %d", len(upstreams))
		}
	})

	t.Run("NewFileProvider with YAML file", func(t *testing.T) {
		tmpDir := t.TempDir()
		yamlFile := filepath.Join(tmpDir, "upstreams.yaml")

		data := UpstreamsFile{
			Upstreams: []config.Upstream{
				{Host: "server1.local", Port: 9000},
				{Host: "server2.local", Port: 9000},
				{Host: "server3.local", Port: 9000},
			},
		}

		yamlData, _ := yaml.Marshal(data)
		if err := os.WriteFile(yamlFile, yamlData, 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		provider, err := NewFileProvider(FileConfig{
			Path:   yamlFile,
			Format: "yaml",
		})

		if err != nil {
			t.Fatalf("failed to create provider: %v", err)
		}
		defer provider.Close()

		ctx := context.Background()
		upstreams, err := provider.Discover(ctx)
		if err != nil {
			t.Fatalf("discover failed: %v", err)
		}

		if len(upstreams) != 3 {
			t.Errorf("expected 3 upstreams, got %d", len(upstreams))
		}
	})

	t.Run("Auto-detect format from extension", func(t *testing.T) {
		tmpDir := t.TempDir()
		jsonFile := filepath.Join(tmpDir, "test.json")

		data := UpstreamsFile{
			Upstreams: []config.Upstream{
				{Host: "auto.local", Port: 8080},
			},
		}

		jsonData, _ := json.Marshal(data)
		if err := os.WriteFile(jsonFile, jsonData, 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		// Don't specify format - should auto-detect
		provider, err := NewFileProvider(FileConfig{
			Path: jsonFile,
		})

		if err != nil {
			t.Fatalf("failed to create provider with auto-detect: %v", err)
		}
		defer provider.Close()

		if provider.format != "json" {
			t.Errorf("expected format 'json', got '%s'", provider.format)
		}
	})

	t.Run("Error on non-existent file", func(t *testing.T) {
		_, err := NewFileProvider(FileConfig{
			Path:   "/nonexistent/path/to/file.json",
			Format: "json",
		})

		if err == nil {
			t.Error("expected error for non-existent file, got nil")
		}
	})

	t.Run("Error on empty upstreams file", func(t *testing.T) {
		tmpDir := t.TempDir()
		emptyFile := filepath.Join(tmpDir, "empty.json")

		data := UpstreamsFile{
			Upstreams: []config.Upstream{},
		}

		jsonData, _ := json.Marshal(data)
		if err := os.WriteFile(emptyFile, jsonData, 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		_, err := NewFileProvider(FileConfig{
			Path:   emptyFile,
			Format: "json",
		})

		if err == nil {
			t.Error("expected error for empty upstreams, got nil")
		}
	})

	t.Run("Error on invalid JSON", func(t *testing.T) {
		tmpDir := t.TempDir()
		badFile := filepath.Join(tmpDir, "bad.json")

		if err := os.WriteFile(badFile, []byte("{invalid json}"), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		_, err := NewFileProvider(FileConfig{
			Path:   badFile,
			Format: "json",
		})

		if err == nil {
			t.Error("expected error for invalid JSON, got nil")
		}
	})

	t.Run("Name returns file basename", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "my-upstreams.json")

		data := UpstreamsFile{
			Upstreams: []config.Upstream{
				{Host: "test.local", Port: 8080},
			},
		}

		jsonData, _ := json.Marshal(data)
		if err := os.WriteFile(testFile, jsonData, 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		provider, err := NewFileProvider(FileConfig{
			Path: testFile,
		})
		if err != nil {
			t.Fatalf("failed to create provider: %v", err)
		}
		defer provider.Close()

		expected := "file:my-upstreams.json"
		if provider.Name() != expected {
			t.Errorf("expected name '%s', got '%s'", expected, provider.Name())
		}
	})

	t.Run("File watch detects changes", func(t *testing.T) {
		tmpDir := t.TempDir()
		watchFile := filepath.Join(tmpDir, "watch.json")

		// Initial data
		initialData := UpstreamsFile{
			Upstreams: []config.Upstream{
				{Host: "initial.local", Port: 8080},
			},
		}

		jsonData, _ := json.Marshal(initialData)
		if err := os.WriteFile(watchFile, jsonData, 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		provider, err := NewFileProvider(FileConfig{
			Path:        watchFile,
			Format:      "json",
			WatchPeriod: "100ms", // Fast refresh for testing
		})
		if err != nil {
			t.Fatalf("failed to create provider: %v", err)
		}
		defer provider.Close()

		ctx := context.Background()
		initialUpstreams, _ := provider.Discover(ctx)
		if len(initialUpstreams) != 1 {
			t.Fatalf("expected 1 initial upstream, got %d", len(initialUpstreams))
		}

		// Update file
		time.Sleep(150 * time.Millisecond) // Wait for initial stat
		updatedData := UpstreamsFile{
			Upstreams: []config.Upstream{
				{Host: "updated1.local", Port: 8080},
				{Host: "updated2.local", Port: 8080},
			},
		}

		jsonData, _ = json.Marshal(updatedData)
		if err := os.WriteFile(watchFile, jsonData, 0644); err != nil {
			t.Fatalf("failed to update test file: %v", err)
		}

		// Wait for file watcher to detect change
		time.Sleep(200 * time.Millisecond)

		updatedUpstreams, _ := provider.Discover(ctx)
		if len(updatedUpstreams) != 2 {
			t.Errorf("expected 2 upstreams after update, got %d", len(updatedUpstreams))
		}
	})

	t.Run("Error on missing path", func(t *testing.T) {
		_, err := NewFileProvider(FileConfig{
			Format: "json",
		})

		if err == nil {
			t.Error("expected error for missing path, got nil")
		}
	})

	t.Run("Error on unsupported format", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "test.txt")
		os.WriteFile(testFile, []byte("test"), 0644)

		_, err := NewFileProvider(FileConfig{
			Path:   testFile,
			Format: "xml",
		})

		if err == nil {
			t.Error("expected error for unsupported format, got nil")
		}
	})
}
