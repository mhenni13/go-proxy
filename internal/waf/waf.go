package waf

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Rule represents a WAF rule
type Rule struct {
	ID          string
	Description string
	Pattern     *regexp.Regexp
	Target      string // "uri", "body", "headers", "query", "all"
	Action      string // "block", "log"
}

// WAF manages web application firewall rules
type WAF struct {
	rules    []Rule
	enabled  bool
	mode     string // "block" or "detect" (log only)
	logFile  *os.File
	logMutex sync.Mutex
}

// NewWAF creates a new WAF instance
func NewWAF(enabled bool, mode string, customRules []RuleConfig) (*WAF, error) {
	return NewWAFWithLog(enabled, mode, customRules, "")
}

// NewWAFWithLog creates a new WAF instance with optional log file
func NewWAFWithLog(enabled bool, mode string, customRules []RuleConfig, logFilePath string) (*WAF, error) {
	waf := &WAF{
		enabled: enabled,
		mode:    mode,
		rules:   []Rule{},
	}

	if !enabled {
		return waf, nil
	}

	// Open log file if specified
	if logFilePath != "" {
		file, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to open WAF log file: %w", err)
		}
		waf.logFile = file
	}

	// Add default rules
	waf.rules = append(waf.rules, getDefaultRules()...)

	// Add custom rules
	for _, ruleConfig := range customRules {
		pattern, err := regexp.Compile(ruleConfig.Pattern)
		if err != nil {
			return nil, err
		}
		waf.rules = append(waf.rules, Rule{
			ID:          ruleConfig.ID,
			Description: ruleConfig.Description,
			Pattern:     pattern,
			Target:      ruleConfig.Target,
			Action:      ruleConfig.Action,
		})
	}

	return waf, nil
}

// Close closes the WAF log file if open
func (w *WAF) Close() error {
	if w.logFile != nil {
		return w.logFile.Close()
	}
	return nil
}

// RuleConfig represents WAF rule configuration
type RuleConfig struct {
	ID          string `yaml:"id"`
	Description string `yaml:"description"`
	Pattern     string `yaml:"pattern"`
	Target      string `yaml:"target"` // "uri", "body", "headers", "query", "all"
	Action      string `yaml:"action"` // "block", "log"
}

// CheckRequest checks if a request violates any WAF rules
func (w *WAF) CheckRequest(r *http.Request, body string) (blocked bool, ruleID string, description string) {
	if !w.enabled {
		return false, "", ""
	}

	for _, rule := range w.rules {
		if w.matchRule(rule, r, body) {
			// In detect mode, we log but don't block
			if w.mode == "detect" {
				return false, rule.ID, rule.Description
			}
			// In block mode, return true to block the request
			return true, rule.ID, rule.Description
		}
	}

	return false, "", ""
}

// matchRule checks if a request matches a specific rule
func (w *WAF) matchRule(rule Rule, r *http.Request, body string) bool {
	switch rule.Target {
	case "uri":
		return rule.Pattern.MatchString(r.URL.Path)

	case "query":
		return rule.Pattern.MatchString(r.URL.RawQuery)

	case "body":
		return rule.Pattern.MatchString(body)

	case "headers":
		for _, values := range r.Header {
			for _, value := range values {
				if rule.Pattern.MatchString(value) {
					return true
				}
			}
		}
		return false

	case "all":
		// Check URI
		if rule.Pattern.MatchString(r.URL.Path) {
			return true
		}
		// Check query
		if rule.Pattern.MatchString(r.URL.RawQuery) {
			return true
		}
		// Check body
		if rule.Pattern.MatchString(body) {
			return true
		}
		// Check headers
		for _, values := range r.Header {
			for _, value := range values {
				if rule.Pattern.MatchString(value) {
					return true
				}
			}
		}
		return false

	default:
		return false
	}
}

// getDefaultRules returns a set of common WAF rules
func getDefaultRules() []Rule {
	return []Rule{
		// SQL Injection patterns
		{
			ID:          "SQL-001",
			Description: "SQL Injection - UNION SELECT",
			Pattern:     regexp.MustCompile(`(?i)(union.*select|select.*from|insert.*into|delete.*from|drop.*table|update.*set)`),
			Target:      "all",
			Action:      "block",
		},
		{
			ID:          "SQL-002",
			Description: "SQL Injection - OR 1=1",
			Pattern:     regexp.MustCompile(`(?i)(\bor\b\s+\d+\s*=\s*\d+|\band\b\s+\d+\s*=\s*\d+|'.*or.*'.*=.*')`),
			Target:      "all",
			Action:      "block",
		},
		{
			ID:          "SQL-003",
			Description: "SQL Injection - Comment sequences",
			Pattern:     regexp.MustCompile(`(--|\/\*|\*\/|;--|#)`),
			Target:      "query",
			Action:      "block",
		},

		// XSS patterns
		{
			ID:          "XSS-001",
			Description: "Cross-Site Scripting - Script tags",
			Pattern:     regexp.MustCompile(`(?i)(<script|<\/script>|javascript:|onerror=|onload=|onclick=|onmouseover=)`),
			Target:      "all",
			Action:      "block",
		},
		{
			ID:          "XSS-002",
			Description: "Cross-Site Scripting - Event handlers",
			Pattern:     regexp.MustCompile(`(?i)(on\w+\s*=|<iframe|<embed|<object)`),
			Target:      "all",
			Action:      "block",
		},

		// Path Traversal
		{
			ID:          "PT-001",
			Description: "Path Traversal - Directory traversal",
			Pattern:     regexp.MustCompile(`(\.\./|\.\.\\|%2e%2e/|%2e%2e\\)`),
			Target:      "uri",
			Action:      "block",
		},
		{
			ID:          "PT-002",
			Description: "Path Traversal - Absolute paths",
			Pattern:     regexp.MustCompile(`(\/etc\/|\/proc\/|\/sys\/|c:\\|\\windows\\)`),
			Target:      "uri",
			Action:      "block",
		},

		// Command Injection
		{
			ID:          "CI-001",
			Description: "Command Injection - Shell commands",
			Pattern:     regexp.MustCompile(`(?i)(;|\||&|>|<|\$\(|` + "`" + `|&&|\|\|)`),
			Target:      "query",
			Action:      "block",
		},

		// File Upload vulnerabilities
		{
			ID:          "FU-001",
			Description: "File Upload - Dangerous extensions",
			Pattern:     regexp.MustCompile(`(?i)\.(exe|sh|bat|cmd|ps1|dll|so|dylib|php|jsp|asp|aspx)$`),
			Target:      "uri",
			Action:      "block",
		},

		// Remote File Inclusion
		{
			ID:          "RFI-001",
			Description: "Remote File Inclusion",
			Pattern:     regexp.MustCompile(`(?i)(https?://|ftp://|file://|data:)`),
			Target:      "query",
			Action:      "block",
		},

		// LDAP Injection
		{
			ID:          "LDAP-001",
			Description: "LDAP Injection",
			Pattern:     regexp.MustCompile(`(\*\)|\(\||&|\(cn=|\(uid=)`),
			Target:      "query",
			Action:      "block",
		},

		// XML/XXE attacks
		{
			ID:          "XXE-001",
			Description: "XML External Entity",
			Pattern:     regexp.MustCompile(`(?i)(<!DOCTYPE|<!ENTITY|SYSTEM|PUBLIC)`),
			Target:      "body",
			Action:      "block",
		},

		// SSRF patterns
		{
			ID:          "SSRF-001",
			Description: "Server-Side Request Forgery",
			Pattern:     regexp.MustCompile(`(?i)(localhost|127\.0\.0\.1|0\.0\.0\.0|::1|169\.254\.|192\.168\.|10\.|172\.(1[6-9]|2[0-9]|3[01])\.)`),
			Target:      "query",
			Action:      "block",
		},

		// Header Injection
		{
			ID:          "HI-001",
			Description: "HTTP Header Injection",
			Pattern:     regexp.MustCompile(`(\r\n|\n\r|%0d%0a|%0a%0d)`),
			Target:      "headers",
			Action:      "block",
		},
	}
}

// IsEnabled returns whether WAF is enabled
func (w *WAF) IsEnabled() bool {
	return w.enabled
}

// GetMode returns the WAF mode
func (w *WAF) GetMode() string {
	return w.mode
}

// SanitizeInput performs basic input sanitization
func SanitizeInput(input string) string {
	// URL decode
	decoded, err := url.QueryUnescape(input)
	if err != nil {
		decoded = input
	}

	// Remove null bytes
	decoded = strings.ReplaceAll(decoded, "\x00", "")

	return decoded
}

// LogEvent logs a WAF event to the log file if configured
func (w *WAF) LogEvent(event map[string]interface{}) {
	if w.logFile == nil {
		return
	}

	w.logMutex.Lock()
	defer w.logMutex.Unlock()

	// Add timestamp
	event["timestamp"] = time.Now().Format(time.RFC3339)

	// Write JSON log entry
	encoder := json.NewEncoder(w.logFile)
	_ = encoder.Encode(event)
}
