package db

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
)

// Bucket names for different data types
const (
	BucketCategories      = "categories"
	BucketTags            = "tags"
	BucketItems           = "items"
	BucketPayments        = "payments"
	BucketFormSubmissions = "form_submissions"
	BucketDownloadLogs    = "download_logs"

	// Index buckets
	BucketPaymentsByInvoice  = "payments_by_invoice"
	BucketPaymentsByStatus   = "payments_by_status"
	BucketItemsByCategory    = "items_by_category"
	BucketItemTags           = "item_tags"
	BucketTagItems           = "tag_items"
	BucketDownloadsByPayment = "downloads_by_payment"
)

// InitBuckets creates all required buckets in the database.
func InitBuckets(db *bolt.DB) error {
	return db.Update(func(tx *bolt.Tx) error {
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

		for _, bucket := range buckets {
			if _, err := tx.CreateBucketIfNotExists([]byte(bucket)); err != nil {
				return fmt.Errorf("failed to create bucket %s: %w", bucket, err)
			}
		}

		return nil
	})
}

// Put stores a value in a bucket with JSON encoding.
func Put(tx *bolt.Tx, bucketName, key string, value interface{}) error {
	bucket := tx.Bucket([]byte(bucketName))
	if bucket == nil {
		return fmt.Errorf("bucket %s not found", bucketName)
	}

	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	return bucket.Put([]byte(key), data)
}

// Get retrieves a value from a bucket with JSON decoding.
func Get(tx *bolt.Tx, bucketName, key string, dest interface{}) error {
	bucket := tx.Bucket([]byte(bucketName))
	if bucket == nil {
		return fmt.Errorf("bucket %s not found", bucketName)
	}

	data := bucket.Get([]byte(key))
	if data == nil {
		return fmt.Errorf("key not found")
	}

	return json.Unmarshal(data, dest)
}

// Delete removes a key from a bucket.
func Delete(tx *bolt.Tx, bucketName, key string) error {
	bucket := tx.Bucket([]byte(bucketName))
	if bucket == nil {
		return fmt.Errorf("bucket %s not found", bucketName)
	}

	return bucket.Delete([]byte(key))
}

// GetAll retrieves all values from a bucket.
func GetAll(tx *bolt.Tx, bucketName string, destSlice interface{}) error {
	bucket := tx.Bucket([]byte(bucketName))
	if bucket == nil {
		return fmt.Errorf("bucket %s not found", bucketName)
	}

	var items []json.RawMessage
	err := bucket.ForEach(func(k, v []byte) error {
		items = append(items, json.RawMessage(v))
		return nil
	})
	if err != nil {
		return err
	}

	// Marshal the items array and unmarshal into the destination
	data, err := json.Marshal(items)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, destSlice)
}

// GetByPrefix retrieves all keys with a specific prefix.
func GetByPrefix(tx *bolt.Tx, bucketName, prefix string, destSlice interface{}) error {
	bucket := tx.Bucket([]byte(bucketName))
	if bucket == nil {
		return fmt.Errorf("bucket %s not found", bucketName)
	}

	var items []json.RawMessage
	c := bucket.Cursor()
	prefixBytes := []byte(prefix)

	for k, v := c.Seek(prefixBytes); k != nil && strings.HasPrefix(string(k), prefix); k, v = c.Next() {
		items = append(items, json.RawMessage(v))
	}

	// Marshal the items array and unmarshal into the destination
	data, err := json.Marshal(items)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, destSlice)
}

// AddIndex adds an index entry (for foreign keys, etc.).
func AddIndex(tx *bolt.Tx, indexBucket, key, value string) error {
	bucket := tx.Bucket([]byte(indexBucket))
	if bucket == nil {
		return fmt.Errorf("bucket %s not found", indexBucket)
	}

	return bucket.Put([]byte(key), []byte(value))
}

// GetIndex retrieves an index entry.
func GetIndex(tx *bolt.Tx, indexBucket, key string) (string, error) {
	bucket := tx.Bucket([]byte(indexBucket))
	if bucket == nil {
		return "", fmt.Errorf("bucket %s not found", indexBucket)
	}

	value := bucket.Get([]byte(key))
	if value == nil {
		return "", fmt.Errorf("index key not found")
	}

	return string(value), nil
}

// DeleteIndex removes an index entry.
func DeleteIndex(tx *bolt.Tx, indexBucket, key string) error {
	bucket := tx.Bucket([]byte(indexBucket))
	if bucket == nil {
		return fmt.Errorf("bucket %s not found", indexBucket)
	}

	return bucket.Delete([]byte(key))
}

// FormatTimestamp formats a time.Time for consistent storage.
func FormatTimestamp(t time.Time) string {
	return t.Format(time.RFC3339Nano)
}

// ParseTimestamp parses a stored timestamp.
func ParseTimestamp(s string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, s)
}
