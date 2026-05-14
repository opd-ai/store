package db

import (
	"path/filepath"
	"testing"

	bolt "go.etcd.io/bbolt"
)

// TestInitBuckets tests bucket initialization
func TestInitBuckets(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	boltDB, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer boltDB.Close()

	// Initialize buckets
	if err := InitBuckets(boltDB); err != nil {
		t.Fatalf("InitBuckets failed: %v", err)
	}

	// Verify all buckets were created
	expectedBuckets := []string{
		BucketCategories,
		BucketTags,
		BucketItems,
		BucketPayments,
		BucketFormSubmissions,
		BucketDownloadLogs,
		BucketPaymentsByInvoice,
		BucketPaymentsByStatus,
		BucketItemsByCategory,
		BucketItemTags,
		BucketTagItems,
		BucketDownloadsByPayment,
	}

	err = boltDB.View(func(tx *bolt.Tx) error {
		for _, bucketName := range expectedBuckets {
			bucket := tx.Bucket([]byte(bucketName))
			if bucket == nil {
				t.Errorf("bucket %s was not created", bucketName)
			}
		}
		return nil
	})

	if err != nil {
		t.Fatalf("failed to verify buckets: %v", err)
	}
}

// TestInitBuckets_Idempotent tests that calling InitBuckets multiple times is safe
func TestInitBuckets_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	boltDB, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer boltDB.Close()

	// Initialize buckets multiple times
	for i := 0; i < 3; i++ {
		if err := InitBuckets(boltDB); err != nil {
			t.Fatalf("InitBuckets failed on iteration %d: %v", i, err)
		}
	}

	// Verify buckets still exist and are functional
	err = boltDB.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(BucketCategories))
		if bucket == nil {
			return ErrBucketNotFound(BucketCategories)
		}
		return bucket.Put([]byte("test"), []byte("value"))
	})

	if err != nil {
		t.Fatalf("failed to use bucket after multiple InitBuckets calls: %v", err)
	}
}

// TestBucketConstants verifies bucket name constants are defined
func TestBucketConstants(t *testing.T) {
	buckets := []string{
		BucketCategories,
		BucketTags,
		BucketItems,
		BucketPayments,
		BucketFormSubmissions,
		BucketDownloadLogs,
		BucketPaymentsByInvoice,
		BucketPaymentsByStatus,
		BucketItemsByCategory,
		BucketItemTags,
		BucketTagItems,
		BucketDownloadsByPayment,
	}

	// Verify no empty bucket names
	for _, name := range buckets {
		if name == "" {
			t.Error("found empty bucket name constant")
		}
	}

	// Verify no duplicate bucket names
	seen := make(map[string]bool)
	for _, name := range buckets {
		if seen[name] {
			t.Errorf("duplicate bucket name: %s", name)
		}
		seen[name] = true
	}
}

// ErrBucketNotFound is a helper for creating bucket not found errors
func ErrBucketNotFound(name string) error {
	return &bucketNotFoundError{name: name}
}

type bucketNotFoundError struct {
	name string
}

func (e *bucketNotFoundError) Error() string {
	return "bucket not found: " + e.name
}
