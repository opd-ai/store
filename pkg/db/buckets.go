package db

import (
	"fmt"

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
