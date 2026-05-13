package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"

	"github.com/opd-ai/store/pkg/db"
	"github.com/opd-ai/store/pkg/handler"
	"github.com/opd-ai/store/pkg/models"
)

// Store orchestrates the payment-to-fulfillment workflow.
// It manages payment creation, confirmation, and handler dispatch.
type Store struct {
	boltDB   *bolt.DB
	registry *handler.Registry
}

// NewStore creates a new Store instance.
func NewStore(boltDB *bolt.DB, registry *handler.Registry) *Store {
	return &Store{
		boltDB:   boltDB,
		registry: registry,
	}
}

// CreatePayment initializes a new payment record.
func (s *Store) CreatePayment(ctx context.Context, itemID, amount, currency string) (*models.Payment, error) {
	payment := models.NewPayment(itemID, amount, currency)

	err := s.boltDB.Update(func(tx *bolt.Tx) error {
		if err := db.Put(tx, db.BucketPayments, payment.ID, payment); err != nil {
			return err
		}
		// Create status index
		return db.AddIndex(tx, db.BucketPaymentsByStatus, payment.Status+":"+payment.ID, payment.ID)
	})

	if err != nil {
		return nil, fmt.Errorf("failed to create payment: %w", err)
	}

	return payment, nil
}

// UpdatePaymentInvoice updates a payment with the invoice ID from the paywall service.
func (s *Store) UpdatePaymentInvoice(ctx context.Context, paymentID, invoiceID string) error {
	return s.boltDB.Update(func(tx *bolt.Tx) error {
		var payment models.Payment
		if err := db.Get(tx, db.BucketPayments, paymentID, &payment); err != nil {
			return fmt.Errorf("payment not found: %w", err)
		}

		payment.InvoiceID = invoiceID
		payment.UpdatedAt = time.Now()

		if err := db.Put(tx, db.BucketPayments, paymentID, &payment); err != nil {
			return fmt.Errorf("failed to update payment: %w", err)
		}

		// Add invoice index
		return db.AddIndex(tx, db.BucketPaymentsByInvoice, invoiceID, paymentID)
	})
}

// UpdatePaymentPayerInfo updates a payment with payer information.
func (s *Store) UpdatePaymentPayerInfo(ctx context.Context, paymentID string, payerInfo models.JSONMap) error {
	return s.boltDB.Update(func(tx *bolt.Tx) error {
		var payment models.Payment
		if err := db.Get(tx, db.BucketPayments, paymentID, &payment); err != nil {
			return fmt.Errorf("payment not found: %w", err)
		}

		payment.PayerInfo = payerInfo
		payment.UpdatedAt = time.Now()

		return db.Put(tx, db.BucketPayments, paymentID, &payment)
	})
}

// updatePaymentField updates a single field on a payment record.
func (s *Store) updatePaymentField(ctx context.Context, paymentID string, field string, value interface{}, errorMsg string) error {
	return s.boltDB.Update(func(tx *bolt.Tx) error {
		var payment models.Payment
		if err := db.Get(tx, db.BucketPayments, paymentID, &payment); err != nil {
			return fmt.Errorf("payment not found: %w", err)
		}

		// Update the specific field using reflection or type switching
		switch field {
		case "invoice_id":
			payment.InvoiceID = value.(string)
		case "payer_info":
			payment.PayerInfo = value.(models.JSONMap)
		default:
			return fmt.Errorf("unknown field: %s", field)
		}

		payment.UpdatedAt = time.Now()

		if err := db.Put(tx, db.BucketPayments, paymentID, &payment); err != nil {
			return fmt.Errorf("%s: %w", errorMsg, err)
		}

		return nil
	})
}

// ConfirmPayment marks a payment as confirmed by the payment gateway.
func (s *Store) ConfirmPayment(ctx context.Context, paymentID, paymentHash string) error {
	return s.boltDB.Update(func(tx *bolt.Tx) error {
		var payment models.Payment
		if err := db.Get(tx, db.BucketPayments, paymentID, &payment); err != nil {
			return fmt.Errorf("payment not found: %w", err)
		}

		// Remove old status index
		db.DeleteIndex(tx, db.BucketPaymentsByStatus, payment.Status+":"+payment.ID)

		now := time.Now()
		payment.Status = "confirmed"
		payment.PaymentHash = &paymentHash
		payment.ConfirmedAt = &now
		payment.UpdatedAt = now

		if err := db.Put(tx, db.BucketPayments, paymentID, &payment); err != nil {
			return fmt.Errorf("failed to confirm payment: %w", err)
		}

		// Add new status index
		return db.AddIndex(tx, db.BucketPaymentsByStatus, payment.Status+":"+payment.ID, payment.ID)
	})
}

// FulfillPayment dispatches the payment to the appropriate handler based on item type.
func (s *Store) FulfillPayment(ctx context.Context, paymentID string) error {
	var payment models.Payment
	var item models.Item

	// Load payment and item
	if err := s.boltDB.View(func(tx *bolt.Tx) error {
		if err := db.Get(tx, db.BucketPayments, paymentID, &payment); err != nil {
			return fmt.Errorf("payment not found: %w", err)
		}

		if err := db.Get(tx, db.BucketItems, payment.ItemID, &item); err != nil {
			return fmt.Errorf("item not found: %w", err)
		}

		return nil
	}); err != nil {
		return err
	}

	// Check if payment is confirmed
	if payment.Status != "confirmed" {
		return fmt.Errorf("payment not confirmed (status: %s)", payment.Status)
	}

	// Get the appropriate handler
	h, err := s.registry.Get(item.BackendType)
	if err != nil {
		return fmt.Errorf("handler for type %q not found: %w", item.BackendType, err)
	}

	// Set item on payment for handler
	payment.Item = &item

	// Execute handler
	result, err := h.Handle(ctx, &payment, &item)
	if err != nil {
		// Update payment with failure status
		s.boltDB.Update(func(tx *bolt.Tx) error {
			db.DeleteIndex(tx, db.BucketPaymentsByStatus, payment.Status+":"+payment.ID)
			payment.Status = "fulfillment_failed"
			payment.UpdatedAt = time.Now()
			db.Put(tx, db.BucketPayments, paymentID, &payment)
			db.AddIndex(tx, db.BucketPaymentsByStatus, payment.Status+":"+payment.ID, payment.ID)
			return nil
		})
		return fmt.Errorf("handler failed: %w", err)
	}

	// Update payment with fulfillment result
	return s.boltDB.Update(func(tx *bolt.Tx) error {
		db.DeleteIndex(tx, db.BucketPaymentsByStatus, payment.Status+":"+payment.ID)

		now := time.Now()
		payment.Status = "fulfilled"
		payment.FulfillmentResult = models.JSONMap(result)
		payment.FulfilledAt = &now
		payment.UpdatedAt = now

		if err := db.Put(tx, db.BucketPayments, paymentID, &payment); err != nil {
			return fmt.Errorf("failed to update fulfillment: %w", err)
		}

		return db.AddIndex(tx, db.BucketPaymentsByStatus, payment.Status+":"+payment.ID, payment.ID)
	})
}

// GetPayment retrieves a single payment by ID.
func (s *Store) GetPayment(ctx context.Context, paymentID string) (*models.Payment, error) {
	var payment models.Payment

	err := s.boltDB.View(func(tx *bolt.Tx) error {
		return db.Get(tx, db.BucketPayments, paymentID, &payment)
	})

	if err != nil {
		return nil, fmt.Errorf("payment not found: %w", err)
	}

	return &payment, nil
}

// GetPaymentByInvoiceID retrieves a single payment by invoice ID.
func (s *Store) GetPaymentByInvoiceID(ctx context.Context, invoiceID string) (*models.Payment, error) {
	var payment models.Payment

	err := s.boltDB.View(func(tx *bolt.Tx) error {
		paymentID, err := db.GetIndex(tx, db.BucketPaymentsByInvoice, invoiceID)
		if err != nil {
			return err
		}

		return db.Get(tx, db.BucketPayments, paymentID, &payment)
	})

	if err != nil {
		return nil, fmt.Errorf("payment not found: %w", err)
	}

	return &payment, nil
}

// ListPayments retrieves a list of payments with optional filtering.
func (s *Store) ListPayments(ctx context.Context, filters map[string]interface{}) ([]*models.Payment, error) {
	var payments []*models.Payment

	err := s.boltDB.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(db.BucketPayments))
		if bucket == nil {
			return fmt.Errorf("payments bucket not found")
		}

		// Get filter values
		statusFilter, hasStatus := filters["status"].(string)
		itemIDFilter, hasItemID := filters["item_id"].(string)

		return bucket.ForEach(func(k, v []byte) error {
			var payment models.Payment
			if err := json.Unmarshal(v, &payment); err != nil {
				return err
			}

			// Apply filters
			if hasStatus && payment.Status != statusFilter {
				return nil
			}
			if hasItemID && payment.ItemID != itemIDFilter {
				return nil
			}

			payments = append(payments, &payment)
			return nil
		})
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list payments: %w", err)
	}

	return payments, nil
}

// UpdateFulfillmentResult updates the fulfillment result for a payment.
func (s *Store) UpdateFulfillmentResult(ctx context.Context, paymentID string, result models.JSONMap) error {
	return s.boltDB.Update(func(tx *bolt.Tx) error {
		var payment models.Payment
		if err := db.Get(tx, db.BucketPayments, paymentID, &payment); err != nil {
			return fmt.Errorf("payment not found: %w", err)
		}

		payment.FulfillmentResult = result
		payment.UpdatedAt = time.Now()

		return db.Put(tx, db.BucketPayments, paymentID, &payment)
	})
}

// GetCatalog retrieves all categories and items for browsing.
func (s *Store) GetCatalog(ctx context.Context) (map[string]interface{}, error) {
	var categories []*models.Category
	var items []*models.Item

	err := s.boltDB.View(func(tx *bolt.Tx) error {
		if err := db.GetAll(tx, db.BucketCategories, &categories); err != nil {
			return fmt.Errorf("failed to load categories: %w", err)
		}

		if err := db.GetAll(tx, db.BucketItems, &items); err != nil {
			return fmt.Errorf("failed to load items: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"categories": categories,
		"items":      items,
	}, nil
}

// GetItem retrieves a single item by ID.
func (s *Store) GetItem(ctx context.Context, itemID string) (*models.Item, error) {
	var item models.Item

	err := s.boltDB.View(func(tx *bolt.Tx) error {
		return db.Get(tx, db.BucketItems, itemID, &item)
	})

	if err != nil {
		return nil, fmt.Errorf("item not found: %w", err)
	}

	return &item, nil
}

// SubmitFormData stores form submission data for a payment.
func (s *Store) SubmitFormData(ctx context.Context, paymentID string, formData models.JSONMap) (*models.FormSubmission, error) {
	submission := models.NewFormSubmission(paymentID, formData)

	err := s.boltDB.Update(func(tx *bolt.Tx) error {
		return db.Put(tx, db.BucketFormSubmissions, submission.ID, submission)
	})

	if err != nil {
		return nil, fmt.Errorf("failed to save form data: %w", err)
	}

	return submission, nil
}

// GetFormSubmission retrieves form data for a payment.
func (s *Store) GetFormSubmission(ctx context.Context, paymentID string) (*models.FormSubmission, error) {
	var submission *models.FormSubmission

	err := s.boltDB.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(db.BucketFormSubmissions))
		if bucket == nil {
			return fmt.Errorf("form_submissions bucket not found")
		}

		return bucket.ForEach(func(k, v []byte) error {
			var sub models.FormSubmission
			if err := json.Unmarshal(v, &sub); err != nil {
				return err
			}

			if sub.PaymentID == paymentID {
				submission = &sub
				return nil
			}

			return nil
		})
	})

	if err != nil {
		return nil, fmt.Errorf("form submission not found: %w", err)
	}

	if submission == nil {
		return nil, fmt.Errorf("form submission not found")
	}

	return submission, nil
}

// HandlerMetadata returns metadata for all registered handlers.
func (s *Store) HandlerMetadata() map[string]handler.HandlerMetadata {
	return s.registry.All()
}

// CreateCategory creates a new category and persists it to the database.
func (s *Store) CreateCategory(ctx context.Context, name, description string) (*models.Category, error) {
	category := models.NewCategory(name, description)

	err := s.boltDB.Update(func(tx *bolt.Tx) error {
		return db.Put(tx, db.BucketCategories, category.ID, category)
	})

	if err != nil {
		return nil, fmt.Errorf("failed to create category: %w", err)
	}

	return category, nil
}

// ListCategories retrieves all categories ordered by order field.
func (s *Store) ListCategories(ctx context.Context) ([]*models.Category, error) {
	var categories []*models.Category

	err := s.boltDB.View(func(tx *bolt.Tx) error {
		return db.GetAll(tx, db.BucketCategories, &categories)
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list categories: %w", err)
	}

	return categories, nil
}

// UpdateCategory updates a category by ID.
func (s *Store) UpdateCategory(ctx context.Context, id string, updates map[string]interface{}) error {
	return s.boltDB.Update(func(tx *bolt.Tx) error {
		var category models.Category
		if err := db.Get(tx, db.BucketCategories, id, &category); err != nil {
			return fmt.Errorf("category not found: %w", err)
		}

		// Apply updates
		if name, ok := updates["name"].(string); ok {
			category.Name = name
		}
		if description, ok := updates["description"].(string); ok {
			category.Description = description
		}
		if order, ok := updates["order"].(int); ok {
			category.Order = order
		}
		if slug, ok := updates["slug"].(string); ok {
			category.Slug = slug
		}
		if metadata, ok := updates["metadata"].(models.JSONMap); ok {
			category.Metadata = metadata
		}

		category.UpdatedAt = time.Now()

		return db.Put(tx, db.BucketCategories, id, &category)
	})
}

// DeleteCategory deletes a category by ID.
func (s *Store) DeleteCategory(ctx context.Context, id string) error {
	return s.boltDB.Update(func(tx *bolt.Tx) error {
		return db.Delete(tx, db.BucketCategories, id)
	})
}

// CreateItem creates a new item and persists it to the database.
func (s *Store) CreateItem(ctx context.Context, item *models.Item) (*models.Item, error) {
	// Validate backend type is registered
	h, err := s.registry.Get(item.BackendType)
	if err != nil {
		return nil, fmt.Errorf("invalid backend_type %q: handler not registered", item.BackendType)
	}

	// Validate backend config
	if err := h.Validate(item.BackendConfig); err != nil {
		return nil, fmt.Errorf("invalid backend_config: %w", err)
	}

	err = s.boltDB.Update(func(tx *bolt.Tx) error {
		if err := db.Put(tx, db.BucketItems, item.ID, item); err != nil {
			return err
		}

		// Add category index
		if item.CategoryID != "" {
			return db.AddIndex(tx, db.BucketItemsByCategory, item.CategoryID+":"+item.ID, item.ID)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to create item: %w", err)
	}

	return item, nil
}

// ListItems retrieves items with optional filtering.
func (s *Store) ListItems(ctx context.Context, filters map[string]interface{}) ([]*models.Item, error) {
	var items []*models.Item

	err := s.boltDB.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(db.BucketItems))
		if bucket == nil {
			return fmt.Errorf("items bucket not found")
		}

		// Get filter values
		categoryIDFilter, hasCategoryID := filters["category_id"].(string)
		backendTypeFilter, hasBackendType := filters["backend_type"].(string)
		activeFilter, hasActive := filters["active"].(bool)

		return bucket.ForEach(func(k, v []byte) error {
			var item models.Item
			if err := json.Unmarshal(v, &item); err != nil {
				return err
			}

			// Apply filters
			if hasCategoryID && item.CategoryID != categoryIDFilter {
				return nil
			}
			if hasBackendType && item.BackendType != backendTypeFilter {
				return nil
			}
			if hasActive && item.Active != activeFilter {
				return nil
			}

			items = append(items, &item)
			return nil
		})
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list items: %w", err)
	}

	return items, nil
}

// UpdateItem updates an item by ID.
func (s *Store) UpdateItem(ctx context.Context, id string, updates map[string]interface{}) error {
	// Validate backend changes if present
	if err := s.validateBackendUpdate(ctx, id, updates); err != nil {
		return err
	}

	return s.boltDB.Update(func(tx *bolt.Tx) error {
		var item models.Item
		if err := db.Get(tx, db.BucketItems, id, &item); err != nil {
			return fmt.Errorf("item not found: %w", err)
		}

		// Apply updates
		if name, ok := updates["name"].(string); ok {
			item.Name = name
		}
		if description, ok := updates["description"].(string); ok {
			item.Description = description
		}
		if price, ok := updates["price"].(string); ok {
			item.Price = price
		}
		if currency, ok := updates["currency"].(string); ok {
			item.Currency = currency
		}
		if image, ok := updates["image"].(string); ok {
			item.Image = image
		}
		if backendType, ok := updates["backend_type"].(string); ok {
			item.BackendType = backendType
		}
		if backendConfig, ok := updates["backend_config"].(models.JSONMap); ok {
			item.BackendConfig = backendConfig
		}
		if metadata, ok := updates["metadata"].(models.JSONMap); ok {
			item.Metadata = metadata
		}
		if active, ok := updates["active"].(bool); ok {
			item.Active = active
		}
		if categoryID, ok := updates["category_id"].(string); ok {
			// Update category index
			if item.CategoryID != "" {
				db.DeleteIndex(tx, db.BucketItemsByCategory, item.CategoryID+":"+item.ID)
			}
			item.CategoryID = categoryID
			if categoryID != "" {
				db.AddIndex(tx, db.BucketItemsByCategory, categoryID+":"+item.ID, item.ID)
			}
		}

		item.UpdatedAt = time.Now()

		return db.Put(tx, db.BucketItems, id, &item)
	})
}

// validateBackendUpdate validates backend_type and backend_config changes.
func (s *Store) validateBackendUpdate(ctx context.Context, itemID string, updates map[string]interface{}) error {
	backendType, hasType := updates["backend_type"].(string)
	backendConfig, hasConfig := updates["backend_config"].(models.JSONMap)

	// No backend changes, no validation needed
	if !hasType && !hasConfig {
		return nil
	}

	validationType, err := s.determineValidationType(ctx, itemID, backendType, hasType)
	if err != nil {
		return err
	}

	h, err := s.registry.Get(validationType)
	if err != nil {
		return fmt.Errorf("invalid backend_type %q: handler not registered", validationType)
	}

	configToValidate, err := s.determineConfigToValidate(ctx, itemID, backendConfig, hasConfig)
	if err != nil {
		return err
	}

	if err := h.Validate(configToValidate); err != nil {
		return fmt.Errorf("invalid backend_config: %w", err)
	}

	return nil
}

// determineValidationType returns the backend type to use for validation.
func (s *Store) determineValidationType(ctx context.Context, itemID string, backendType string, hasType bool) (string, error) {
	if hasType {
		return backendType, nil
	}

	item, err := s.GetItem(ctx, itemID)
	if err != nil {
		return "", fmt.Errorf("item not found: %w", err)
	}

	return item.BackendType, nil
}

// determineConfigToValidate returns the backend config to validate.
func (s *Store) determineConfigToValidate(ctx context.Context, itemID string, backendConfig models.JSONMap, hasConfig bool) (models.JSONMap, error) {
	if hasConfig {
		return backendConfig, nil
	}

	item, err := s.GetItem(ctx, itemID)
	if err != nil {
		return nil, fmt.Errorf("item not found: %w", err)
	}

	return item.BackendConfig, nil
}

// DeleteItem soft-deletes an item by setting active to false.
func (s *Store) DeleteItem(ctx context.Context, id string) error {
	return s.UpdateItem(ctx, id, map[string]interface{}{"active": false})
}

// CreateTag creates a new tag and persists it to the database.
func (s *Store) CreateTag(ctx context.Context, name string) (*models.Tag, error) {
	tag := models.NewTag(name)

	err := s.boltDB.Update(func(tx *bolt.Tx) error {
		return db.Put(tx, db.BucketTags, tag.ID, tag)
	})

	if err != nil {
		return nil, fmt.Errorf("failed to create tag: %w", err)
	}

	return tag, nil
}

// ListTags retrieves all tags.
func (s *Store) ListTags(ctx context.Context) ([]*models.Tag, error) {
	var tags []*models.Tag

	err := s.boltDB.View(func(tx *bolt.Tx) error {
		return db.GetAll(tx, db.BucketTags, &tags)
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list tags: %w", err)
	}

	return tags, nil
}

// UpdateTag updates a tag by ID.
func (s *Store) UpdateTag(ctx context.Context, id string, updates map[string]interface{}) error {
	return s.boltDB.Update(func(tx *bolt.Tx) error {
		var tag models.Tag
		if err := db.Get(tx, db.BucketTags, id, &tag); err != nil {
			return fmt.Errorf("tag not found: %w", err)
		}

		// Apply updates
		if name, ok := updates["name"].(string); ok {
			tag.Name = name
		}
		if slug, ok := updates["slug"].(string); ok {
			tag.Slug = slug
		}

		return db.Put(tx, db.BucketTags, id, &tag)
	})
}

// DeleteTag deletes a tag by ID.
func (s *Store) DeleteTag(ctx context.Context, id string) error {
	return s.boltDB.Update(func(tx *bolt.Tx) error {
		// Note: In a real implementation, you'd want to also clean up
		// item-tag associations here
		return db.Delete(tx, db.BucketTags, id)
	})
}

// AddItemTag associates a tag with an item.
func (s *Store) AddItemTag(ctx context.Context, itemID, tagID string) error {
	return s.modifyItemTagAssociation(ctx, itemID, tagID, true)
}

// RemoveItemTag removes a tag association from an item.
func (s *Store) RemoveItemTag(ctx context.Context, itemID, tagID string) error {
	return s.modifyItemTagAssociation(ctx, itemID, tagID, false)
}

// modifyItemTagAssociation handles adding or removing tag associations.
func (s *Store) modifyItemTagAssociation(ctx context.Context, itemID, tagID string, add bool) error {
	return s.boltDB.Update(func(tx *bolt.Tx) error {
		// Verify item exists
		var item models.Item
		if err := db.Get(tx, db.BucketItems, itemID, &item); err != nil {
			return fmt.Errorf("item not found: %w", err)
		}

		// Verify tag exists
		var tag models.Tag
		if err := db.Get(tx, db.BucketTags, tagID, &tag); err != nil {
			return fmt.Errorf("tag not found: %w", err)
		}

		// Modify associations in index buckets
		itemTagKey := itemID + ":" + tagID
		tagItemKey := tagID + ":" + itemID

		if add {
			if err := db.AddIndex(tx, db.BucketItemTags, itemTagKey, tagID); err != nil {
				return fmt.Errorf("failed to add item-tag association: %w", err)
			}
			if err := db.AddIndex(tx, db.BucketTagItems, tagItemKey, itemID); err != nil {
				return fmt.Errorf("failed to add tag-item association: %w", err)
			}
		} else {
			if err := db.DeleteIndex(tx, db.BucketItemTags, itemTagKey); err != nil {
				return fmt.Errorf("failed to remove item-tag association: %w", err)
			}
			if err := db.DeleteIndex(tx, db.BucketTagItems, tagItemKey); err != nil {
				return fmt.Errorf("failed to remove tag-item association: %w", err)
			}
		}

		return nil
	})
}

// RecordDownload records a download attempt for rate limiting.
func (s *Store) RecordDownload(ctx context.Context, paymentID, ipAddress, userAgent string) error {
	downloadLog := models.NewDownloadLog(paymentID, ipAddress, userAgent)

	return s.boltDB.Update(func(tx *bolt.Tx) error {
		if err := db.Put(tx, db.BucketDownloadLogs, downloadLog.ID, downloadLog); err != nil {
			return fmt.Errorf("failed to record download: %w", err)
		}

		// Add payment index
		indexKey := paymentID + ":" + downloadLog.ID
		return db.AddIndex(tx, db.BucketDownloadsByPayment, indexKey, downloadLog.ID)
	})
}

// GetDownloadCount returns the number of downloads for a payment.
func (s *Store) GetDownloadCount(ctx context.Context, paymentID string) (int, error) {
	count := 0

	err := s.boltDB.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(db.BucketDownloadsByPayment))
		if bucket == nil {
			return nil
		}

		prefix := paymentID + ":"
		c := bucket.Cursor()

		for k, _ := c.Seek([]byte(prefix)); k != nil && strings.HasPrefix(string(k), prefix); k, _ = c.Next() {
			count++
		}

		return nil
	})

	if err != nil {
		return 0, fmt.Errorf("failed to get download count: %w", err)
	}

	return count, nil
}

// CheckDownloadLimit checks if a payment has exceeded its download limit.
func (s *Store) CheckDownloadLimit(ctx context.Context, paymentID string, maxDownloads int) (bool, error) {
	if maxDownloads <= 0 {
		return false, nil // Unlimited downloads
	}

	count, err := s.GetDownloadCount(ctx, paymentID)
	if err != nil {
		return false, err
	}

	return count >= maxDownloads, nil
}
