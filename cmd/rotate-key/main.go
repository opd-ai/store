// Package main provides a utility for rotating encryption keys in the store database.
// This tool re-encrypts all backend_config data with a new encryption key,
// enabling zero-downtime key rotation for security compliance.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
	bolt "go.etcd.io/bbolt"

	"github.com/opd-ai/store/pkg/crypto"
	"github.com/opd-ai/store/pkg/db"
	"github.com/opd-ai/store/pkg/models"
)

func main() {
	var (
		dbPath      string
		oldKey      string
		newKey      string
		generateKey bool
	)

	flag.StringVar(&dbPath, "db", "", "Path to BoltDB database file")
	flag.StringVar(&oldKey, "old-key", "", "Old encryption key (base64)")
	flag.StringVar(&newKey, "new-key", "", "New encryption key (base64)")
	flag.BoolVar(&generateKey, "generate", false, "Generate a new encryption key")
	flag.Parse()

	// Generate key mode
	if generateKey {
		key, err := crypto.GenerateKeyBase64()
		if err != nil {
			log.Fatalf("Failed to generate key: %v", err)
		}
		fmt.Printf("Generated encryption key (base64):\n%s\n", key)
		fmt.Println("\nAdd this to your environment:")
		fmt.Printf("export STORE_ENCRYPTION_KEY=%s\n", key)
		return
	}

	// Load .env file if present
	_ = godotenv.Load()

	// Get database path
	if dbPath == "" {
		dbPath = os.Getenv("STORE_DATABASE_PATH")
		if dbPath == "" {
			dbPath = "./data/store.db"
		}
	}

	// Get old key from env if not provided
	if oldKey == "" {
		oldKey = os.Getenv("STORE_ENCRYPTION_KEY")
	}

	// Validate inputs
	if newKey == "" {
		log.Fatal("Error: --new-key is required for key rotation")
	}

	// Initialize encryption services
	var oldEncryption *crypto.EncryptionService
	var err error

	if oldKey != "" {
		oldEncryption, err = crypto.NewEncryptionServiceFromBase64(oldKey)
		if err != nil {
			log.Fatalf("Failed to initialize old encryption service: %v", err)
		}
		log.Println("Old encryption key loaded")
	} else {
		log.Println("Warning: No old key provided, assuming data is currently unencrypted")
	}

	newEncryption, err := crypto.NewEncryptionServiceFromBase64(newKey)
	if err != nil {
		log.Fatalf("Failed to initialize new encryption service: %v", err)
	}
	log.Println("New encryption key loaded")

	// Open database
	boltDB, err := bolt.Open(dbPath, 0o600, &bolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer boltDB.Close()

	database := db.NewBoltDatabase(boltDB)

	// Perform key rotation
	if err := rotateKeys(context.Background(), database, oldEncryption, newEncryption); err != nil {
		log.Fatalf("Key rotation failed: %v", err)
	}

	log.Println("Key rotation completed successfully!")
	log.Println("\nUpdate your environment:")
	log.Printf("export STORE_ENCRYPTION_KEY=%s\n", newKey)
}

// rotateKeys performs the key rotation by re-encrypting all items' backend configs.
func rotateKeys(ctx context.Context, database db.Database, oldEncryption, newEncryption *crypto.EncryptionService) error {
	log.Println("Starting key rotation...")

	var items []*models.Item

	// Read all items
	err := database.View(func(tx db.Transaction) error {
		return tx.GetBucket(db.BucketItems).GetAll(&items)
	})
	if err != nil {
		return fmt.Errorf("failed to read items: %w", err)
	}

	log.Printf("Found %d items to process\n", len(items))

	processed := 0
	skipped := 0

	// Process each item
	for _, item := range items {
		if len(item.BackendConfig) == 0 {
			skipped++
			continue
		}

		// Decrypt with old key (or use plaintext if no old key)
		var plainConfig models.JSONMap
		if oldEncryption != nil {
			decrypted, err := decryptBackendConfig(oldEncryption, item.BackendConfig)
			if err != nil {
				// If decryption fails, assume it's already plaintext
				log.Printf("Item %s: Treating config as plaintext (decryption failed: %v)\n", item.ID, err)
				plainConfig = item.BackendConfig
			} else {
				plainConfig = decrypted
			}
		} else {
			// No old key, assume plaintext
			plainConfig = item.BackendConfig
		}

		// Encrypt with new key
		encryptedConfig, err := encryptBackendConfig(newEncryption, plainConfig)
		if err != nil {
			return fmt.Errorf("failed to encrypt item %s: %w", item.ID, err)
		}

		// Update item in database
		err = database.Update(func(tx db.Transaction) error {
			item.BackendConfig = encryptedConfig
			item.UpdatedAt = time.Now()
			return tx.GetBucket(db.BucketItems).Put(item.ID, item)
		})
		if err != nil {
			return fmt.Errorf("failed to update item %s: %w", item.ID, err)
		}

		processed++
		if processed%10 == 0 {
			log.Printf("Processed %d/%d items...\n", processed, len(items))
		}
	}

	log.Printf("Rotation complete: %d items processed, %d skipped (empty config)\n", processed, skipped)
	return nil
}

// encryptBackendConfig encrypts a backend configuration.
func encryptBackendConfig(encryption *crypto.EncryptionService, config models.JSONMap) (models.JSONMap, error) {
	// Marshal config to JSON
	plaintext, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}

	// Encrypt
	ciphertext, err := encryption.Encrypt(plaintext)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt: %w", err)
	}

	// Return as JSONMap with encrypted data and flag
	return models.JSONMap{
		"_encrypted": true,
		"_data":      ciphertext,
	}, nil
}

// decryptBackendConfig decrypts a backend configuration.
func decryptBackendConfig(encryption *crypto.EncryptionService, config models.JSONMap) (models.JSONMap, error) {
	// Check if config is encrypted
	encrypted, ok := config["_encrypted"].(bool)
	if !ok || !encrypted {
		// Not encrypted, return as-is
		return config, nil
	}

	// Extract encrypted data
	dataRaw, ok := config["_data"]
	if !ok {
		return nil, fmt.Errorf("encrypted config missing _data field")
	}

	// Convert data to []byte
	var ciphertext []byte
	switch v := dataRaw.(type) {
	case []byte:
		ciphertext = v
	case string:
		ciphertext = []byte(v)
	default:
		return nil, fmt.Errorf("invalid encrypted data type: %T", dataRaw)
	}

	// Decrypt
	plaintext, err := encryption.Decrypt(ciphertext)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	// Unmarshal back to JSONMap
	var decrypted models.JSONMap
	if err := json.Unmarshal(plaintext, &decrypted); err != nil {
		return nil, fmt.Errorf("failed to unmarshal decrypted config: %w", err)
	}

	return decrypted, nil
}
