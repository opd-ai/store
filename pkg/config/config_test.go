package config

import (
"os"
"path/filepath"
"testing"
"time"
)

func TestLoad_Defaults(t *testing.T) {
cfg, err := Load("")
if err != nil {
t.Fatalf("Load() error = %v", err)
}

// Check default values
if cfg.ServerPort != "8080" {
t.Errorf("ServerPort = %v, want 8080", cfg.ServerPort)
}
if cfg.DatabasePath != "./data/store.db" {
t.Errorf("DatabasePath = %v, want ./data/store.db", cfg.DatabasePath)
}
if cfg.RateLimitEnabled != true {
t.Errorf("RateLimitEnabled = %v, want true", cfg.RateLimitEnabled)
}
if cfg.RateLimitRPM != 60 {
t.Errorf("RateLimitRPM = %v, want 60", cfg.RateLimitRPM)
}
if cfg.CSRFEnabled != true {
t.Errorf("CSRFEnabled = %v, want true", cfg.CSRFEnabled)
}
}

func TestLoad_EnvironmentOverrides(t *testing.T) {
// Set environment variables
os.Setenv("STORE_PORT", "9090")
os.Setenv("STORE_DB_PATH", "/tmp/test.db")
os.Setenv("STORE_RATE_LIMIT_RPM", "120")
defer func() {
os.Unsetenv("STORE_PORT")
os.Unsetenv("STORE_DB_PATH")
os.Unsetenv("STORE_RATE_LIMIT_RPM")
}()

cfg, err := Load("")
if err != nil {
t.Fatalf("Load() error = %v", err)
}

// Check environment overrides
if cfg.ServerPort != "9090" {
t.Errorf("ServerPort = %v, want 9090", cfg.ServerPort)
}
if cfg.DatabasePath != "/tmp/test.db" {
t.Errorf("DatabasePath = %v, want /tmp/test.db", cfg.DatabasePath)
}
if cfg.RateLimitRPM != 120 {
t.Errorf("RateLimitRPM = %v, want 120", cfg.RateLimitRPM)
}
}

func TestLoad_ConfigFile(t *testing.T) {
// Create a temporary config file
tmpDir := t.TempDir()
configPath := filepath.Join(tmpDir, "config.yaml")

configContent := `
server_port: "3000"
database_path: "/custom/db.db"
paywall_url: "http://paywall.example.com"
rate_limit_rpm: 200
rate_limit_burst: 20
csrf_enabled: false
`

if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
t.Fatalf("Failed to create config file: %v", err)
}

cfg, err := Load(configPath)
if err != nil {
t.Fatalf("Load() error = %v", err)
}

// Check config file values
if cfg.ServerPort != "3000" {
t.Errorf("ServerPort = %v, want 3000", cfg.ServerPort)
}
if cfg.DatabasePath != "/custom/db.db" {
t.Errorf("DatabasePath = %v, want /custom/db.db", cfg.DatabasePath)
}
if cfg.PaywallURL != "http://paywall.example.com" {
t.Errorf("PaywallURL = %v, want http://paywall.example.com", cfg.PaywallURL)
}
if cfg.RateLimitRPM != 200 {
t.Errorf("RateLimitRPM = %v, want 200", cfg.RateLimitRPM)
}
if cfg.RateLimitBurst != 20 {
t.Errorf("RateLimitBurst = %v, want 20", cfg.RateLimitBurst)
}
if cfg.CSRFEnabled != false {
t.Errorf("CSRFEnabled = %v, want false", cfg.CSRFEnabled)
}
}

func TestLoad_EnvironmentOverridesConfigFile(t *testing.T) {
// Create a temporary config file
tmpDir := t.TempDir()
configPath := filepath.Join(tmpDir, "config.yaml")

configContent := `
server_port: "3000"
database_path: "/custom/db.db"
rate_limit_rpm: 200
`

if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
t.Fatalf("Failed to create config file: %v", err)
}

// Set environment variable that should override config file
os.Setenv("STORE_PORT", "4000")
defer os.Unsetenv("STORE_PORT")

cfg, err := Load(configPath)
if err != nil {
t.Fatalf("Load() error = %v", err)
}

// Environment variable should take precedence
if cfg.ServerPort != "4000" {
t.Errorf("ServerPort = %v, want 4000 (env should override config file)", cfg.ServerPort)
}
// Config file value should still be used for non-overridden values
if cfg.DatabasePath != "/custom/db.db" {
t.Errorf("DatabasePath = %v, want /custom/db.db", cfg.DatabasePath)
}
}

func TestLoad_InvalidConfigFile(t *testing.T) {
_, err := Load("/nonexistent/config.yaml")
if err == nil {
t.Error("Load() expected error for nonexistent config file, got nil")
}
}

func TestLoad_Timeouts(t *testing.T) {
cfg, err := Load("")
if err != nil {
t.Fatalf("Load() error = %v", err)
}

// Check timeout defaults
if cfg.ReadTimeout != 10*time.Second {
t.Errorf("ReadTimeout = %v, want 10s", cfg.ReadTimeout)
}
if cfg.WriteTimeout != 10*time.Second {
t.Errorf("WriteTimeout = %v, want 10s", cfg.WriteTimeout)
}
if cfg.ShutdownTimeout != 30*time.Second {
t.Errorf("ShutdownTimeout = %v, want 30s", cfg.ShutdownTimeout)
}
}

func TestLoad_BooleanEnvironmentVariables(t *testing.T) {
// Test "false" string disables features
os.Setenv("STORE_ENCRYPTION_ENABLED", "false")
os.Setenv("STORE_CSRF_ENABLED", "false")
os.Setenv("STORE_RATE_LIMIT_ENABLED", "false")
defer func() {
os.Unsetenv("STORE_ENCRYPTION_ENABLED")
os.Unsetenv("STORE_CSRF_ENABLED")
os.Unsetenv("STORE_RATE_LIMIT_ENABLED")
}()

cfg, err := Load("")
if err != nil {
t.Fatalf("Load() error = %v", err)
}

if cfg.EncryptionEnabled != false {
t.Errorf("EncryptionEnabled = %v, want false", cfg.EncryptionEnabled)
}
if cfg.CSRFEnabled != false {
t.Errorf("CSRFEnabled = %v, want false", cfg.CSRFEnabled)
}
if cfg.RateLimitEnabled != false {
t.Errorf("RateLimitEnabled = %v, want false", cfg.RateLimitEnabled)
}
}

func TestLoad_CORSOrigins(t *testing.T) {
// Create a temporary config file with multiple CORS origins
tmpDir := t.TempDir()
configPath := filepath.Join(tmpDir, "config.yaml")

configContent := `
cors_origins:
  - "https://example.com"
  - "https://app.example.com"
`

if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
t.Fatalf("Failed to create config file: %v", err)
}

cfg, err := Load(configPath)
if err != nil {
t.Fatalf("Load() error = %v", err)
}

if len(cfg.CORSOrigins) != 2 {
t.Errorf("len(CORSOrigins) = %v, want 2", len(cfg.CORSOrigins))
}
if cfg.CORSOrigins[0] != "https://example.com" {
t.Errorf("CORSOrigins[0] = %v, want https://example.com", cfg.CORSOrigins[0])
}
if cfg.CORSOrigins[1] != "https://app.example.com" {
t.Errorf("CORSOrigins[1] = %v, want https://app.example.com", cfg.CORSOrigins[1])
}
}
