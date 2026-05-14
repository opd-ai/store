package db

import (
	"context"
)

// Database abstracts database operations for the store.
type Database interface {
	// View executes a read-only transaction.
	View(fn func(Transaction) error) error

	// Update executes a read-write transaction.
	Update(fn func(Transaction) error) error

	// Close closes the database connection.
	Close() error
}

// Transaction represents a database transaction.
type Transaction interface {
	// GetBucket retrieves a bucket by name.
	GetBucket(name string) Bucket

	// Context returns the transaction context.
	Context() context.Context
}

// Bucket represents a key-value bucket within a transaction.
type Bucket interface {
	// Get retrieves a value by key and unmarshals it into dest.
	Get(key string, dest interface{}) error

	// Put stores a value at the given key.
	Put(key string, value interface{}) error

	// Delete removes a value by key.
	Delete(key string) error

	// GetAll retrieves all values in the bucket.
	GetAll(destSlice interface{}) error

	// GetByPrefix retrieves all values with keys matching the prefix.
	GetByPrefix(prefix string, destSlice interface{}) error

	// AddIndex adds an index entry mapping key to value.
	AddIndex(indexName, indexKey, value string) error

	// GetIndex retrieves the value for an index key.
	GetIndex(indexName, indexKey string) (string, error)

	// DeleteIndex removes an index entry.
	DeleteIndex(indexName, indexKey string) error
}
