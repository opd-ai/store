// Package db provides database abstraction for the store.
// It implements a key-value database interface using BoltDB with support for
// namespaced buckets (categories, items, payments, tags, forms, downloads, audit logs).
//
// Key types: Database, BoltDatabase.
package db

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	bolt "go.etcd.io/bbolt"
)

// BoltDatabase is a BoltDB implementation of the Database interface.
type BoltDatabase struct {
	db *bolt.DB
}

// NewBoltDatabase creates a new BoltDatabase instance.
func NewBoltDatabase(db *bolt.DB) *BoltDatabase {
	return &BoltDatabase{db: db}
}

// View executes a read-only transaction.
func (b *BoltDatabase) View(fn func(Transaction) error) error {
	return b.db.View(func(tx *bolt.Tx) error {
		return fn(&BoltTransaction{tx: tx, ctx: context.Background()})
	})
}

// Update executes a read-write transaction.
func (b *BoltDatabase) Update(fn func(Transaction) error) error {
	return b.db.Update(func(tx *bolt.Tx) error {
		return fn(&BoltTransaction{tx: tx, ctx: context.Background()})
	})
}

// Close closes the database connection.
func (b *BoltDatabase) Close() error {
	return b.db.Close()
}

// BoltTransaction is a BoltDB implementation of the Transaction interface.
type BoltTransaction struct {
	tx  *bolt.Tx
	ctx context.Context
}

// GetBucket retrieves a bucket by name.
func (bt *BoltTransaction) GetBucket(name string) Bucket {
	return &BoltBucket{
		tx:         bt.tx,
		bucketName: name,
	}
}

// Context returns the transaction context.
func (bt *BoltTransaction) Context() context.Context {
	return bt.ctx
}

// BoltBucket is a BoltDB implementation of the Bucket interface.
type BoltBucket struct {
	tx         *bolt.Tx
	bucketName string
}

// Get retrieves a value by key and unmarshals it into dest.
func (bb *BoltBucket) Get(key string, dest interface{}) error {
	bucket := bb.tx.Bucket([]byte(bb.bucketName))
	if bucket == nil {
		return fmt.Errorf("bucket %s not found", bb.bucketName)
	}

	data := bucket.Get([]byte(key))
	if data == nil {
		return fmt.Errorf("key %s not found in bucket %s", key, bb.bucketName)
	}

	return json.Unmarshal(data, dest)
}

// Put stores a value at the given key.
func (bb *BoltBucket) Put(key string, value interface{}) error {
	bucket := bb.tx.Bucket([]byte(bb.bucketName))
	if bucket == nil {
		return fmt.Errorf("bucket %s not found", bb.bucketName)
	}

	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	return bucket.Put([]byte(key), data)
}

// Delete removes a value by key.
func (bb *BoltBucket) Delete(key string) error {
	bucket := bb.tx.Bucket([]byte(bb.bucketName))
	if bucket == nil {
		return fmt.Errorf("bucket %s not found", bb.bucketName)
	}

	return bucket.Delete([]byte(key))
}

// GetAll retrieves all values in the bucket.
func (bb *BoltBucket) GetAll(destSlice interface{}) error {
	bucket := bb.tx.Bucket([]byte(bb.bucketName))
	if bucket == nil {
		return fmt.Errorf("bucket %s not found", bb.bucketName)
	}

	slice := reflect.ValueOf(destSlice)
	if slice.Kind() != reflect.Ptr || slice.Elem().Kind() != reflect.Slice {
		return fmt.Errorf("destSlice must be a pointer to a slice")
	}

	sliceElem := slice.Elem()
	elemType := sliceElem.Type().Elem()

	c := bucket.Cursor()
	for k, v := c.First(); k != nil; k, v = c.Next() {
		item := reflect.New(elemType).Interface()
		if err := json.Unmarshal(v, item); err != nil {
			return fmt.Errorf("failed to unmarshal item: %w", err)
		}
		sliceElem.Set(reflect.Append(sliceElem, reflect.ValueOf(item).Elem()))
	}

	return nil
}

// GetByPrefix retrieves all values with keys matching the prefix.
func (bb *BoltBucket) GetByPrefix(prefix string, destSlice interface{}) error {
	bucket := bb.tx.Bucket([]byte(bb.bucketName))
	if bucket == nil {
		return fmt.Errorf("bucket %s not found", bb.bucketName)
	}

	slice := reflect.ValueOf(destSlice)
	if slice.Kind() != reflect.Ptr || slice.Elem().Kind() != reflect.Slice {
		return fmt.Errorf("destSlice must be a pointer to a slice")
	}

	sliceElem := slice.Elem()
	elemType := sliceElem.Type().Elem()
	prefixBytes := []byte(prefix)

	c := bucket.Cursor()
	for k, v := c.Seek(prefixBytes); k != nil && hasPrefix(k, prefixBytes); k, v = c.Next() {
		item := reflect.New(elemType).Interface()
		if err := json.Unmarshal(v, item); err != nil {
			return fmt.Errorf("failed to unmarshal item: %w", err)
		}
		sliceElem.Set(reflect.Append(sliceElem, reflect.ValueOf(item).Elem()))
	}

	return nil
}

// AddIndex adds an index entry mapping key to value.
func (bb *BoltBucket) AddIndex(indexName, indexKey, value string) error {
	indexBucket := bb.tx.Bucket([]byte(indexName))
	if indexBucket == nil {
		return fmt.Errorf("index bucket %s not found", indexName)
	}

	return indexBucket.Put([]byte(indexKey), []byte(value))
}

// GetIndex retrieves the value for an index key.
func (bb *BoltBucket) GetIndex(indexName, indexKey string) (string, error) {
	indexBucket := bb.tx.Bucket([]byte(indexName))
	if indexBucket == nil {
		return "", fmt.Errorf("index bucket %s not found", indexName)
	}

	value := indexBucket.Get([]byte(indexKey))
	if value == nil {
		return "", fmt.Errorf("index key %s not found in bucket %s", indexKey, indexName)
	}

	return string(value), nil
}

// DeleteIndex removes an index entry.
func (bb *BoltBucket) DeleteIndex(indexName, indexKey string) error {
	indexBucket := bb.tx.Bucket([]byte(indexName))
	if indexBucket == nil {
		return fmt.Errorf("index bucket %s not found", indexName)
	}

	return indexBucket.Delete([]byte(indexKey))
}

// hasPrefix checks if a byte slice has the given prefix.
func hasPrefix(s, prefix []byte) bool {
	if len(s) < len(prefix) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		if s[i] != prefix[i] {
			return false
		}
	}
	return true
}
