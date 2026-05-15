// Package config provides application configuration management.
// It supports loading configuration from files (YAML), environment variables,
// and defaults. Priority order: CLI flags > env vars > config file > defaults.
//
// Example usage:
//
//	cfg, err := config.Load("config.yaml")
//	if err != nil {
//		log.Fatal(err)
//	}
//	server := &http.Server{Addr: ":" + cfg.ServerPort}
package config

import (
	"fmt"
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

	// Embedded Paywall settings
	PaywallTestnet          bool          `mapstructure:"paywall_testnet"`
	PaywallDBPath           string        `mapstructure:"paywall_db_path"`
	PaywallTimeout          time.Duration `mapstructure:"paywall_timeout"`
	PaywallMinConfirmations int           `mapstructure:"paywall_min_confirmations"`

	// Payment modes per handler type
	PaymentModeDigital  string `mapstructure:"payment_mode_digital"`
	PaymentModeShipping string `mapstructure:"payment_mode_shipping"`
	PaymentModePOD      string `mapstructure:"payment_mode_pod"`

	// Multisig/Escrow configuration
	MultisigEnabled       bool          `mapstructure:"multisig_enabled"`
	SellerPublicKey       string        `mapstructure:"seller_public_key"`
	ArbiterPublicKey      string        `mapstructure:"arbiter_public_key"`
	SellerPrivateKey      string        `mapstructure:"seller_private_key"`
	EscrowTimeoutPhysical time.Duration `mapstructure:"escrow_timeout_physical"`

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

	// Background job settings
	PoDPollingEnabled  bool          `mapstructure:"pod_polling_enabled"`
	PoDPollingInterval time.Duration `mapstructure:"pod_polling_interval"`
}

// Load loads configuration from file, environment variables, and defaults
// Priority order: CLI flags > environment variables > config file > defaults
func Load(configFile string) (*Config, error) {
	v := viper.New()

	// Set defaults
	setDefaults(v)

	// Set up environment variable reading with explicit bindings
	v.SetEnvPrefix("STORE")
	v.AutomaticEnv()
	bindEnvironmentVariables(v)

	// Load config file if specified
	if configFile != "" {
		v.SetConfigFile(configFile)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
	}

	// Unmarshal into Config struct
	// Environment variables take precedence due to viper's priority order
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

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

	// Embedded Paywall defaults
	v.SetDefault("paywall_testnet", true)
	v.SetDefault("paywall_db_path", "./data/paywall.db")
	v.SetDefault("paywall_timeout", 24*time.Hour)
	v.SetDefault("paywall_min_confirmations", 1)

	// Payment mode defaults
	v.SetDefault("payment_mode_digital", "single-sig")
	v.SetDefault("payment_mode_shipping", "multisig-escrow")
	v.SetDefault("payment_mode_pod", "single-sig")

	// Multisig/Escrow defaults
	v.SetDefault("multisig_enabled", false)
	v.SetDefault("seller_public_key", "")
	v.SetDefault("arbiter_public_key", "")
	v.SetDefault("seller_private_key", "")
	v.SetDefault("escrow_timeout_physical", 7*24*time.Hour)

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

	// Background job defaults
	v.SetDefault("pod_polling_enabled", true)
	v.SetDefault("pod_polling_interval", 1*time.Hour)
}

// bindEnvironmentVariables explicitly binds environment variables to config keys
// This ensures STORE_* env vars are properly mapped to config struct fields
func bindEnvironmentVariables(v *viper.Viper) {
	// Map of environment variable suffix to config key
	envBindings := map[string]string{
		// Server settings
		"PORT":             "server_port",
		"READ_TIMEOUT":     "read_timeout",
		"WRITE_TIMEOUT":    "write_timeout",
		"SHUTDOWN_TIMEOUT": "shutdown_timeout",

		// Database settings
		"DB_PATH":       "database_path",
		"DATABASE_PATH": "database_path",

		// Embedded Paywall settings
		"PAYWALL_TESTNET":           "paywall_testnet",
		"PAYWALL_DB_PATH":           "paywall_db_path",
		"PAYWALL_TIMEOUT":           "paywall_timeout",
		"PAYWALL_MIN_CONFIRMATIONS": "paywall_min_confirmations",

		// Payment modes
		"PAYMENT_MODE_DIGITAL":  "payment_mode_digital",
		"PAYMENT_MODE_SHIPPING": "payment_mode_shipping",
		"PAYMENT_MODE_POD":      "payment_mode_pod",

		// Multisig/Escrow settings
		"MULTISIG_ENABLED":        "multisig_enabled",
		"SELLER_PUBLIC_KEY":       "seller_public_key",
		"ARBITER_PUBLIC_KEY":      "arbiter_public_key",
		"SELLER_PRIVATE_KEY":      "seller_private_key",
		"ESCROW_TIMEOUT_PHYSICAL": "escrow_timeout_physical",

		// Encryption settings
		"ENCRYPTION_ENABLED": "encryption_enabled",
		"ENCRYPTION_KEY":     "encryption_key",

		// Rate limiting
		"RATE_LIMIT_ENABLED": "rate_limit_enabled",
		"RATE_LIMIT_RPM":     "rate_limit_rpm",
		"RATE_LIMIT_BURST":   "rate_limit_burst",

		// CSRF protection
		"CSRF_ENABLED": "csrf_enabled",

		// Admin API
		"ADMIN_TOKEN": "admin_token",

		// Audit log settings
		"AUDIT_LOG_RETENTION_DAYS": "audit_log_retention_days",

		// CORS settings
		"CORS_ORIGINS": "cors_origins",

		// Background job settings
		"POD_POLLING_ENABLED":  "pod_polling_enabled",
		"POD_POLLING_INTERVAL": "pod_polling_interval",
	}

	// Bind each environment variable
	for envSuffix, configKey := range envBindings {
		v.BindEnv(configKey, "STORE_"+envSuffix)
	}
}
