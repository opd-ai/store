package config

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/viper"
)

// Config holds all application configuration
type Config struct {
	// Server settings
	ServerPort      string        `mapstructure:"server_port"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`

	// Database settings
	DatabasePath string `mapstructure:"database_path"`

	// Paywall settings
	PaywallURL        string `mapstructure:"paywall_url"`
	PaywallWebhookKey string `mapstructure:"paywall_webhook_key"`

	// Encryption settings
	EncryptionEnabled bool   `mapstructure:"encryption_enabled"`
	EncryptionKey     string `mapstructure:"encryption_key"`

	// Rate limiting
	RateLimitEnabled bool `mapstructure:"rate_limit_enabled"`
	RateLimitRPM     int  `mapstructure:"rate_limit_rpm"`
	RateLimitBurst   int  `mapstructure:"rate_limit_burst"`

	// CSRF protection
	CSRFEnabled bool `mapstructure:"csrf_enabled"`

	// Admin API
	AdminToken string `mapstructure:"admin_token"`

	// Audit log settings
	AuditLogRetentionDays int `mapstructure:"audit_log_retention_days"`

	// CORS settings
	CORSOrigins []string `mapstructure:"cors_origins"`
}

// Load loads configuration from file, environment variables, and defaults
// Priority order: CLI flags > environment variables > config file > defaults
func Load(configFile string) (*Config, error) {
	v := viper.New()

	// Set defaults
	setDefaults(v)

	// Set up environment variable reading
	v.SetEnvPrefix("STORE")
	v.AutomaticEnv()

	// Load config file if specified
	if configFile != "" {
		v.SetConfigFile(configFile)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
	}

	// Unmarshal into Config struct
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	// Override with environment variables if present
	// This ensures ENV vars take precedence over config file
	applyEnvironmentOverrides(&cfg)

	return &cfg, nil
}

// setDefaults sets default values for all configuration options
func setDefaults(v *viper.Viper) {
	// Server defaults
	v.SetDefault("server_port", "8080")
	v.SetDefault("read_timeout", 10*time.Second)
	v.SetDefault("write_timeout", 10*time.Second)
	v.SetDefault("shutdown_timeout", 30*time.Second)

	// Database defaults
	v.SetDefault("database_path", "./data/store.db")

	// Paywall defaults
	v.SetDefault("paywall_url", "http://localhost:8081")
	v.SetDefault("paywall_webhook_key", "")

	// Encryption defaults
	v.SetDefault("encryption_enabled", true)
	v.SetDefault("encryption_key", "")

	// Rate limiting defaults
	v.SetDefault("rate_limit_enabled", true)
	v.SetDefault("rate_limit_rpm", 60)
	v.SetDefault("rate_limit_burst", 10)

	// CSRF defaults
	v.SetDefault("csrf_enabled", true)

	// Admin defaults
	v.SetDefault("admin_token", "")

	// Audit log defaults
	v.SetDefault("audit_log_retention_days", 90)

	// CORS defaults
	v.SetDefault("cors_origins", []string{"*"})
}

// applyEnvironmentOverrides ensures environment variables take precedence
func applyEnvironmentOverrides(cfg *Config) {
	// Server settings
	if port := os.Getenv("STORE_PORT"); port != "" {
		cfg.ServerPort = port
	}

	// Database settings
	if dbPath := os.Getenv("STORE_DB_PATH"); dbPath != "" {
		cfg.DatabasePath = dbPath
	}

	// Paywall settings
	if paywallURL := os.Getenv("STORE_PAYWALL_URL"); paywallURL != "" {
		cfg.PaywallURL = paywallURL
	}
	if webhookKey := os.Getenv("STORE_PAYWALL_WEBHOOK_KEY"); webhookKey != "" {
		cfg.PaywallWebhookKey = webhookKey
	}

	// Encryption settings
	if encEnabled := os.Getenv("STORE_ENCRYPTION_ENABLED"); encEnabled != "" {
		cfg.EncryptionEnabled = encEnabled != "false"
	}
	if encKey := os.Getenv("STORE_ENCRYPTION_KEY"); encKey != "" {
		cfg.EncryptionKey = encKey
	}

	// Rate limiting
	if rlEnabled := os.Getenv("STORE_RATE_LIMIT_ENABLED"); rlEnabled != "" {
		cfg.RateLimitEnabled = rlEnabled != "false"
	}
	if rlRPM := os.Getenv("STORE_RATE_LIMIT_RPM"); rlRPM != "" {
		fmt.Sscanf(rlRPM, "%d", &cfg.RateLimitRPM)
	}
	if rlBurst := os.Getenv("STORE_RATE_LIMIT_BURST"); rlBurst != "" {
		fmt.Sscanf(rlBurst, "%d", &cfg.RateLimitBurst)
	}

	// CSRF protection
	if csrfEnabled := os.Getenv("STORE_CSRF_ENABLED"); csrfEnabled != "" {
		cfg.CSRFEnabled = csrfEnabled != "false"
	}

	// Admin token
	if adminToken := os.Getenv("STORE_ADMIN_TOKEN"); adminToken != "" {
		cfg.AdminToken = adminToken
	}

	// Audit log retention
	if retentionDays := os.Getenv("STORE_AUDIT_LOG_RETENTION_DAYS"); retentionDays != "" {
		fmt.Sscanf(retentionDays, "%d", &cfg.AuditLogRetentionDays)
	}
}
