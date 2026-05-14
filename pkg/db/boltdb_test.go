package db

import (
	"path/filepath"
	"sync"
	"testing"

	bolt "go.etcd.io/bbolt"
)

// TestBoltDatabase_NewBoltDatabase tests database creation
func TestBoltDatabase_NewBoltDatabase(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	if db == nil {
		t.Fatal("expected non-nil database")
	}
}

// TestBoltDatabase_View tests read-only transactions
func TestBoltDatabase_View(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	// First, insert some test data
	testKey := "test-key"
	testValue := map[string]string{"name": "test"}

	err := db.Update(func(tx Transaction) error {
		bucket := tx.GetBucket(BucketCategories)
		return bucket.Put(testKey, testValue)
	})
	if err != nil {
		t.Fatalf("failed to insert test data: %v", err)
	}

	// Now test View
	var result map[string]string
	err = db.View(func(tx Transaction) error {
		bucket := tx.GetBucket(BucketCategories)
		return bucket.Get(testKey, &result)
	})

	if err != nil {
		t.Fatalf("View transaction failed: %v", err)
	}

	if result["name"] != "test" {
		t.Errorf("expected name=test, got %v", result["name"])
	}
}

// TestBoltDatabase_Update tests read-write transactions
func TestBoltDatabase_Update(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	testData := []struct {
		key   string
		value map[string]string
	}{
		{"key1", map[string]string{"name": "value1"}},
		{"key2", map[string]string{"name": "value2"}},
	}

	// Insert multiple items in one transaction
	err := db.Update(func(tx Transaction) error {
		bucket := tx.GetBucket(BucketCategories)
		for _, td := range testData {
			if err := bucket.Put(td.key, td.value); err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		t.Fatalf("Update transaction failed: %v", err)
	}

	// Verify data was written
	for _, td := range testData {
		var result map[string]string
		err := db.View(func(tx Transaction) error {
			bucket := tx.GetBucket(BucketCategories)
			return bucket.Get(td.key, &result)
		})
		if err != nil {
			t.Errorf("failed to read key %s: %v", td.key, err)
		}
		if result["name"] != td.value["name"] {
			t.Errorf("expected %s, got %s", td.value["name"], result["name"])
		}
	}
}

// TestBoltDatabase_Close tests database closure
func TestBoltDatabase_Close(t *testing.T) {
	db := setupTestDB(t)

	if err := db.Close(); err != nil {
		t.Fatalf("failed to close database: %v", err)
	}

	// Attempting View after close should fail
	err := db.View(func(tx Transaction) error {
		return nil
	})
	if err == nil {
		t.Error("expected error when viewing closed database")
	}
}

// TestBoltBucket_Put tests storing values
func TestBoltBucket_Put(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	testCases := []struct {
		name      string
		key       string
		value     interface{}
		expectErr bool
	}{
		{
			name:      "simple map",
			key:       "test1",
			value:     map[string]string{"field": "value"},
			expectErr: false,
		},
		{
			name:      "struct",
			key:       "test2",
			value:     struct{ Name string }{Name: "test"},
			expectErr: false,
		},
		{
			name:      "nil value marshals as null",
			key:       "test3",
			value:     nil,
			expectErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := db.Update(func(tx Transaction) error {
				bucket := tx.GetBucket(BucketCategories)
				return bucket.Put(tc.key, tc.value)
			})

			if tc.expectErr && err == nil {
				t.Error("expected error but got none")
			}
			if !tc.expectErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestBoltBucket_Get tests retrieving values
func TestBoltBucket_Get(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	// Setup test data
	testKey := "get-test"
	testValue := map[string]interface{}{"name": "test", "count": float64(42)}

	err := db.Update(func(tx Transaction) error {
		bucket := tx.GetBucket(BucketCategories)
		return bucket.Put(testKey, testValue)
	})
	if err != nil {
		t.Fatalf("failed to setup test data: %v", err)
	}

	// Test successful get
	t.Run("successful get", func(t *testing.T) {
		var result map[string]interface{}
		err := db.View(func(tx Transaction) error {
			bucket := tx.GetBucket(BucketCategories)
			return bucket.Get(testKey, &result)
		})
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if result["name"] != "test" {
			t.Errorf("expected name=test, got %v", result["name"])
		}
		if result["count"] != float64(42) {
			t.Errorf("expected count=42, got %v", result["count"])
		}
	})

	// Test get non-existent key
	t.Run("non-existent key", func(t *testing.T) {
		var result map[string]string
		err := db.View(func(tx Transaction) error {
			bucket := tx.GetBucket(BucketCategories)
			return bucket.Get("nonexistent", &result)
		})
		if err == nil {
			t.Error("expected error for non-existent key")
		}
	})

	// Test get with non-existent bucket
	t.Run("non-existent bucket", func(t *testing.T) {
		var result map[string]string
		err := db.View(func(tx Transaction) error {
			bucket := tx.GetBucket("nonexistent-bucket")
			return bucket.Get(testKey, &result)
		})
		if err == nil {
			t.Error("expected error for non-existent bucket")
		}
	})
}

// TestBoltBucket_Delete tests removing values
func TestBoltBucket_Delete(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	testKey := "delete-test"
	testValue := map[string]string{"name": "test"}

	// Insert test data
	err := db.Update(func(tx Transaction) error {
		bucket := tx.GetBucket(BucketCategories)
		return bucket.Put(testKey, testValue)
	})
	if err != nil {
		t.Fatalf("failed to insert test data: %v", err)
	}

	// Delete the data
	err = db.Update(func(tx Transaction) error {
		bucket := tx.GetBucket(BucketCategories)
		return bucket.Delete(testKey)
	})
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify deletion
	var result map[string]string
	err = db.View(func(tx Transaction) error {
		bucket := tx.GetBucket(BucketCategories)
		return bucket.Get(testKey, &result)
	})
	if err == nil {
		t.Error("expected error when getting deleted key")
	}
}

// TestBoltBucket_GetAll tests retrieving all values
func TestBoltBucket_GetAll(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	// Insert multiple items
	testData := []map[string]string{
		{"id": "1", "name": "item1"},
		{"id": "2", "name": "item2"},
		{"id": "3", "name": "item3"},
	}

	err := db.Update(func(tx Transaction) error {
		bucket := tx.GetBucket(BucketCategories)
		for i, item := range testData {
			if err := bucket.Put(string(rune('a'+i)), item); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed to insert test data: %v", err)
	}

	// Retrieve all items
	var results []map[string]string
	err = db.View(func(tx Transaction) error {
		bucket := tx.GetBucket(BucketCategories)
		return bucket.GetAll(&results)
	})
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}

	if len(results) != len(testData) {
		t.Errorf("expected %d items, got %d", len(testData), len(results))
	}

	// Test invalid destination type
	t.Run("invalid destination", func(t *testing.T) {
		var invalidDest string
		err := db.View(func(tx Transaction) error {
			bucket := tx.GetBucket(BucketCategories)
			return bucket.GetAll(&invalidDest)
		})
		if err == nil {
			t.Error("expected error for invalid destination type")
		}
	})
}

// TestBoltBucket_GetByPrefix tests prefix-based retrieval
func TestBoltBucket_GetByPrefix(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	// Insert items with different prefixes
	testData := map[string]map[string]string{
		"user:1":  {"name": "alice"},
		"user:2":  {"name": "bob"},
		"admin:1": {"name": "charlie"},
		"user:3":  {"name": "dave"},
	}

	err := db.Update(func(tx Transaction) error {
		bucket := tx.GetBucket(BucketCategories)
		for key, value := range testData {
			if err := bucket.Put(key, value); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed to insert test data: %v", err)
	}

	// Get items with "user:" prefix
	var userResults []map[string]string
	err = db.View(func(tx Transaction) error {
		bucket := tx.GetBucket(BucketCategories)
		return bucket.GetByPrefix("user:", &userResults)
	})
	if err != nil {
		t.Fatalf("GetByPrefix failed: %v", err)
	}

	if len(userResults) != 3 {
		t.Errorf("expected 3 user items, got %d", len(userResults))
	}

	// Get items with "admin:" prefix
	var adminResults []map[string]string
	err = db.View(func(tx Transaction) error {
		bucket := tx.GetBucket(BucketCategories)
		return bucket.GetByPrefix("admin:", &adminResults)
	})
	if err != nil {
		t.Fatalf("GetByPrefix failed: %v", err)
	}

	if len(adminResults) != 1 {
		t.Errorf("expected 1 admin item, got %d", len(adminResults))
	}
}

// TestBoltBucket_AddIndex tests adding index entries
func TestBoltBucket_AddIndex(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	err := db.Update(func(tx Transaction) error {
		bucket := tx.GetBucket(BucketCategories)
		return bucket.AddIndex(BucketPaymentsByInvoice, "invoice-123", "payment-456")
	})

	if err != nil {
		t.Fatalf("AddIndex failed: %v", err)
	}

	// Verify index was added
	var value string
	err = db.View(func(tx Transaction) error {
		bucket := tx.GetBucket(BucketCategories)
		value, err = bucket.GetIndex(BucketPaymentsByInvoice, "invoice-123")
		return err
	})

	if err != nil {
		t.Fatalf("GetIndex failed: %v", err)
	}

	if value != "payment-456" {
		t.Errorf("expected payment-456, got %s", value)
	}
}

// TestBoltBucket_GetIndex tests retrieving index entries
func TestBoltBucket_GetIndex(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	// Add test index
	err := db.Update(func(tx Transaction) error {
		bucket := tx.GetBucket(BucketCategories)
		return bucket.AddIndex(BucketPaymentsByInvoice, "test-invoice", "test-payment")
	})
	if err != nil {
		t.Fatalf("failed to add test index: %v", err)
	}

	// Test successful get
	t.Run("successful get", func(t *testing.T) {
		var value string
		var getErr error
		err := db.View(func(tx Transaction) error {
			bucket := tx.GetBucket(BucketCategories)
			value, getErr = bucket.GetIndex(BucketPaymentsByInvoice, "test-invoice")
			return getErr
		})
		if err != nil {
			t.Fatalf("GetIndex failed: %v", err)
		}
		if value != "test-payment" {
			t.Errorf("expected test-payment, got %s", value)
		}
	})

	// Test non-existent index key
	t.Run("non-existent key", func(t *testing.T) {
		err := db.View(func(tx Transaction) error {
			bucket := tx.GetBucket(BucketCategories)
			_, getErr := bucket.GetIndex(BucketPaymentsByInvoice, "nonexistent")
			return getErr
		})
		if err == nil {
			t.Error("expected error for non-existent index key")
		}
	})
}

// TestBoltBucket_DeleteIndex tests removing index entries
func TestBoltBucket_DeleteIndex(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	// Add test index
	err := db.Update(func(tx Transaction) error {
		bucket := tx.GetBucket(BucketCategories)
		return bucket.AddIndex(BucketPaymentsByInvoice, "delete-invoice", "delete-payment")
	})
	if err != nil {
		t.Fatalf("failed to add test index: %v", err)
	}

	// Delete the index
	err = db.Update(func(tx Transaction) error {
		bucket := tx.GetBucket(BucketCategories)
		return bucket.DeleteIndex(BucketPaymentsByInvoice, "delete-invoice")
	})
	if err != nil {
		t.Fatalf("DeleteIndex failed: %v", err)
	}

	// Verify deletion
	err = db.View(func(tx Transaction) error {
		bucket := tx.GetBucket(BucketCategories)
		_, getErr := bucket.GetIndex(BucketPaymentsByInvoice, "delete-invoice")
		return getErr
	})
	if err == nil {
		t.Error("expected error when getting deleted index")
	}
}

// TestBoltTransaction_Context tests transaction context
func TestBoltTransaction_Context(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	err := db.View(func(tx Transaction) error {
		ctx := tx.Context()
		if ctx == nil {
			t.Error("expected non-nil context")
		}
		return nil
	})

	if err != nil {
		t.Fatalf("transaction failed: %v", err)
	}
}

// TestConcurrentTransactions tests thread safety
func TestConcurrentTransactions(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	const goroutines = 10
	const iterations = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)

	// Concurrent writes
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				key := string(rune('a' + id*iterations + j))
				value := map[string]int{"id": id, "iter": j}
				err := db.Update(func(tx Transaction) error {
					bucket := tx.GetBucket(BucketCategories)
					return bucket.Put(key, value)
				})
				if err != nil {
					t.Errorf("concurrent write failed: %v", err)
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify all writes succeeded
	var results []map[string]int
	err := db.View(func(tx Transaction) error {
		bucket := tx.GetBucket(BucketCategories)
		return bucket.GetAll(&results)
	})
	if err != nil {
		t.Fatalf("failed to read results: %v", err)
	}

	expectedCount := goroutines * iterations
	if len(results) != expectedCount {
		t.Errorf("expected %d items, got %d", expectedCount, len(results))
	}
}

// TestInvalidJSON tests handling of malformed JSON
func TestInvalidJSON(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	// Insert valid data first
	testKey := "invalid-json-test"
	err := db.Update(func(tx Transaction) error {
		bucket := tx.GetBucket(BucketCategories)
		// Manually insert invalid JSON by accessing the underlying bolt bucket
		boltBucket := bucket.(*BoltBucket).tx.Bucket([]byte(BucketCategories))
		return boltBucket.Put([]byte(testKey), []byte("{invalid json}"))
	})
	if err != nil {
		t.Fatalf("failed to insert invalid JSON: %v", err)
	}

	// Try to read the invalid JSON
	var result map[string]string
	err = db.View(func(tx Transaction) error {
		bucket := tx.GetBucket(BucketCategories)
		return bucket.Get(testKey, &result)
	})

	if err == nil {
		t.Error("expected error when reading invalid JSON")
	}
}

// TestUnmarshalableType tests marshaling of unmarshalable types
func TestUnmarshalableType(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	// Channels cannot be marshaled to JSON
	invalidValue := make(chan int)

	err := db.Update(func(tx Transaction) error {
		bucket := tx.GetBucket(BucketCategories)
		return bucket.Put("test", invalidValue)
	})

	if err == nil {
		t.Error("expected error when marshaling unmarshalable type")
	}
}

// TestHasPrefix tests the hasPrefix helper function
func TestHasPrefix(t *testing.T) {
	testCases := []struct {
		s      []byte
		prefix []byte
		want   bool
	}{
		{[]byte("hello"), []byte("hel"), true},
		{[]byte("hello"), []byte("hello"), true},
		{[]byte("hello"), []byte("world"), false},
		{[]byte("hi"), []byte("hello"), false},
		{[]byte(""), []byte(""), true},
		{[]byte("test"), []byte(""), true},
	}

	for _, tc := range testCases {
		got := hasPrefix(tc.s, tc.prefix)
		if got != tc.want {
			t.Errorf("hasPrefix(%q, %q) = %v, want %v", tc.s, tc.prefix, got, tc.want)
		}
	}
}

// setupTestDB creates a temporary database for testing
func setupTestDB(t *testing.T) *BoltDatabase {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	boltDB, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	// Initialize buckets
	if err := InitBuckets(boltDB); err != nil {
		boltDB.Close()
		t.Fatalf("failed to initialize buckets: %v", err)
	}

	return NewBoltDatabase(boltDB)
}

// cleanupTestDB closes and removes the test database
func cleanupTestDB(t *testing.T, db *BoltDatabase) {
	t.Helper()

	if err := db.Close(); err != nil {
		t.Errorf("failed to close test database: %v", err)
	}
}
