package crypto

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestNewEncryptionService(t *testing.T) {
	key := make([]byte, 32)
	_, err := NewEncryptionService(key)
	if err != nil {
		t.Fatalf("NewEncryptionService() error = %v", err)
	}

	// Test invalid key length
	invalidKey := make([]byte, 16)
	_, err = NewEncryptionService(invalidKey)
	if err == nil {
		t.Error("NewEncryptionService() should fail with invalid key length")
	}
}

func TestEncryptDecrypt(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	svc, err := NewEncryptionService(key)
	if err != nil {
		t.Fatalf("Failed to create encryption service: %v", err)
	}

	plaintext := []byte("test data")
	ciphertext, err := svc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	decrypted, err := svc.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("Decrypted data does not match original")
	}
}

func TestGenerateKey(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}

	if len(key) != 32 {
		t.Errorf("Generated key length = %d, want 32", len(key))
	}
}

func TestGenerateKeyBase64(t *testing.T) {
	keyB64, err := GenerateKeyBase64()
	if err != nil {
		t.Fatalf("GenerateKeyBase64() error = %v", err)
	}

	key, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		t.Errorf("Generated key is not valid base64: %v", err)
	}

	if len(key) != 32 {
		t.Errorf("Decoded key length = %d, want 32", len(key))
	}
}
