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

	// Embedded Paywall settings
	if testnet := os.Getenv("STORE_PAYWALL_TESTNET"); testnet != "" {
		cfg.PaywallTestnet = testnet != "false"
	}
	if pwDBPath := os.Getenv("STORE_PAYWALL_DB_PATH"); pwDBPath != "" {
		cfg.PaywallDBPath = pwDBPath
	}
	if pwTimeout := os.Getenv("STORE_PAYWALL_TIMEOUT"); pwTimeout != "" {
		if duration, err := time.ParseDuration(pwTimeout); err == nil {
			cfg.PaywallTimeout = duration
		}
	}

	// Payment mode settings
	if modeDigital := os.Getenv("STORE_PAYMENT_MODE_DIGITAL"); modeDigital != "" {
		cfg.PaymentModeDigital = modeDigital
	}
	if modeShipping := os.Getenv("STORE_PAYMENT_MODE_SHIPPING"); modeShipping != "" {
		cfg.PaymentModeShipping = modeShipping
	}
	if modePOD := os.Getenv("STORE_PAYMENT_MODE_POD"); modePOD != "" {
		cfg.PaymentModePOD = modePOD
	}

	// Multisig/Escrow settings
	if multisigEnabled := os.Getenv("STORE_MULTISIG_ENABLED"); multisigEnabled != "" {
		cfg.MultisigEnabled = multisigEnabled != "false"
	}
	if sellerPubKey := os.Getenv("STORE_SELLER_PUBLIC_KEY"); sellerPubKey != "" {
		cfg.SellerPublicKey = sellerPubKey
	}
	if arbiterPubKey := os.Getenv("STORE_ARBITER_PUBLIC_KEY"); arbiterPubKey != "" {
		cfg.ArbiterPublicKey = arbiterPubKey
	}
	if sellerPrivKey := os.Getenv("STORE_SELLER_PRIVATE_KEY"); sellerPrivKey != "" {
		cfg.SellerPrivateKey = sellerPrivKey
	}
	if escrowTimeout := os.Getenv("STORE_ESCROW_TIMEOUT_PHYSICAL"); escrowTimeout != "" {
		if duration, err := time.ParseDuration(escrowTimeout); err == nil {
			cfg.EscrowTimeoutPhysical = duration
		}
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

	// Background jobs
	if pollingEnabled := os.Getenv("STORE_POD_POLLING_ENABLED"); pollingEnabled != "" {
		cfg.PoDPollingEnabled = pollingEnabled != "false"
	}
	if pollingInterval := os.Getenv("STORE_POD_POLLING_INTERVAL"); pollingInterval != "" {
		if duration, err := time.ParseDuration(pollingInterval); err == nil {
			cfg.PoDPollingInterval = duration
		}
	}
	// Audit log retention
	if retentionDays := os.Getenv("STORE_AUDIT_LOG_RETENTION_DAYS"); retentionDays != "" {
		fmt.Sscanf(retentionDays, "%d", &cfg.AuditLogRetentionDays)
	}
}
