// Package store provides the core business logic for payment orchestration and fulfillment.
// It coordinates payments, catalog management, fulfillment handlers, and audit logging.
//
// Key types: Service (interface), Store (implementation).
//
// Example usage:
//
//	store := store.NewStore(database, registry)
//	payment, err := store.CreatePayment(ctx, itemID, amount, currency)
//	err = store.ConfirmPayment(ctx, paymentID, hash)
//	err = store.FulfillPayment(ctx, paymentID)
package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/opd-ai/store/pkg/crypto"
	"github.com/opd-ai/store/pkg/db"
	"github.com/opd-ai/store/pkg/handler"
	"github.com/opd-ai/store/pkg/metrics"
	"github.com/opd-ai/store/pkg/models"
)

// Service defines the interface for the core store operations.
type Service interface {
	// Payment operations
	CreatePayment(ctx context.Context, itemID, amount, currency string) (*models.Payment, error)
	UpdatePaymentInvoice(ctx context.Context, paymentID, invoiceID string) error
	UpdatePaymentPayerInfo(ctx context.Context, paymentID string, payerInfo models.JSONMap) error
	ConfirmPayment(ctx context.Context, paymentID, paymentHash string) error
	FulfillPayment(ctx context.Context, paymentID string) error
	GetPayment(ctx context.Context, paymentID string) (*models.Payment, error)
	GetPaymentByInvoiceID(ctx context.Context, invoiceID string) (*models.Payment, error)
	ListPayments(ctx context.Context, filters map[string]interface{}) ([]*models.Payment, error)
	UpdateFulfillmentResult(ctx context.Context, paymentID string, result models.JSONMap) error

	// Catalog operations
	GetCatalog(ctx context.Context) (map[string]interface{}, error)
	GetItem(ctx context.Context, itemID string) (*models.Item, error)

	// Form submission operations
	SubmitFormData(ctx context.Context, paymentID string, formData models.JSONMap) (*models.FormSubmission, error)
	GetFormSubmission(ctx context.Context, paymentID string) (*models.FormSubmission, error)

	// Handler operations
	HandlerMetadata() map[string]handler.HandlerMetadata

	// Category operations
	CreateCategory(ctx context.Context, name, description string) (*models.Category, error)
	ListCategories(ctx context.Context) ([]*models.Category, error)
	UpdateCategory(ctx context.Context, id string, updates map[string]interface{}) error
	DeleteCategory(ctx context.Context, id string) error

	// Item operations
	CreateItem(ctx context.Context, item *models.Item) (*models.Item, error)
	ListItems(ctx context.Context, filters map[string]interface{}) ([]*models.Item, error)
	UpdateItem(ctx context.Context, id string, updates map[string]interface{}) error
	DeleteItem(ctx context.Context, id string) error

	// Tag operations
	CreateTag(ctx context.Context, name string) (*models.Tag, error)
	ListTags(ctx context.Context) ([]*models.Tag, error)
	UpdateTag(ctx context.Context, id string, updates map[string]interface{}) error
	DeleteTag(ctx context.Context, id string) error

	// Item-Tag association operations
	AddItemTag(ctx context.Context, itemID, tagID string) error
	RemoveItemTag(ctx context.Context, itemID, tagID string) error

	// Download tracking operations
	RecordDownload(ctx context.Context, paymentID, ipAddress, userAgent string) error
	GetDownloadCount(ctx context.Context, paymentID string) (int, error)
	CheckDownloadLimit(ctx context.Context, paymentID string, maxDownloads int) (bool, error)

	// Audit log operations
	CreateAuditLog(ctx context.Context, log *models.AuditLog) error
	ListAuditLogs(ctx context.Context, filters map[string]interface{}) ([]*models.AuditLog, error)
	CleanupOldAuditLogs(ctx context.Context, retentionDays int) (int, error)

	// Escrow operations
	UpdateEscrowState(ctx context.Context, paymentID string, newState string, additionalData models.JSONMap) error
	UpdateEscrowSignatures(ctx context.Context, paymentID string, signatures []models.EscrowSignature) error
	UpdateEscrowDispute(ctx context.Context, paymentID string, reason string) error
	UpdateEscrowResolution(ctx context.Context, paymentID string, resolution string) error
}

// Store orchestrates the payment-to-fulfillment workflow.
// It manages payment creation, confirmation, and handler dispatch.
type Store struct {
	database   db.Database
	registry   handler.HandlerRegistry
	encryption *crypto.EncryptionService // Optional encryption for sensitive config data
}

// Verify that Store implements Service at compile time.
var _ Service = (*Store)(nil)

// NewStore creates a new Store instance.
func NewStore(database db.Database, registry handler.HandlerRegistry) *Store {
	return &Store{
		database:   database,
		registry:   registry,
		encryption: nil, // Encryption disabled by default
	}
}

// SetEncryption sets the encryption service for encrypting sensitive configuration data.
func (s *Store) SetEncryption(encryption *crypto.EncryptionService) {
	s.encryption = encryption
}

// CreatePayment initializes a new payment record.
func (s *Store) CreatePayment(ctx context.Context, itemID, amount, currency string) (*models.Payment, error) {
	payment := models.NewPayment(itemID, amount, currency)

	err := s.database.Update(func(tx db.Transaction) error {
		if err := tx.GetBucket(db.BucketPayments).Put(payment.ID, payment); err != nil {
			return err
		}
		// Create status index
		return tx.GetBucket(db.BucketPayments).AddIndex(db.BucketPaymentsByStatus, payment.Status+":"+payment.ID, payment.ID)
	})
	if err != nil {
		metrics.RecordPayment("failed")
		return nil, fmt.Errorf("failed to create payment: %w", err)
	}

	metrics.RecordPayment("pending")
	return payment, nil
}

// UpdatePaymentInvoice updates a payment with the invoice ID from the paywall service.
func (s *Store) UpdatePaymentInvoice(ctx context.Context, paymentID, invoiceID string) error {
	return s.database.Update(func(tx db.Transaction) error {
		var payment models.Payment
		if err := tx.GetBucket(db.BucketPayments).Get(paymentID, &payment); err != nil {
			return fmt.Errorf("payment not found: %w", err)
		}

		payment.InvoiceID = invoiceID
		payment.UpdatedAt = time.Now()

		if err := tx.GetBucket(db.BucketPayments).Put(paymentID, &payment); err != nil {
			return fmt.Errorf("failed to update payment: %w", err)
		}

		// Add invoice index
		return tx.GetBucket(db.BucketPayments).AddIndex(db.BucketPaymentsByInvoice, invoiceID, paymentID)
	})
}

// UpdatePaymentPayerInfo updates a payment with payer information.
func (s *Store) UpdatePaymentPayerInfo(ctx context.Context, paymentID string, payerInfo models.JSONMap) error {
	return s.database.Update(func(tx db.Transaction) error {
		var payment models.Payment
		if err := tx.GetBucket(db.BucketPayments).Get(paymentID, &payment); err != nil {
			return fmt.Errorf("payment not found: %w", err)
		}

		payment.PayerInfo = payerInfo
		payment.UpdatedAt = time.Now()

		return tx.GetBucket(db.BucketPayments).Put(paymentID, &payment)
	})
}

// updatePaymentField updates a single field on a payment record.
func (s *Store) updatePaymentField(ctx context.Context, paymentID, field string, value interface{}, errorMsg string) error {
	return s.database.Update(func(tx db.Transaction) error {
		var payment models.Payment
		if err := tx.GetBucket(db.BucketPayments).Get(paymentID, &payment); err != nil {
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

		if err := tx.GetBucket(db.BucketPayments).Put(paymentID, &payment); err != nil {
			return fmt.Errorf("%s: %w", errorMsg, err)
		}

		return nil
	})
}

// ConfirmPayment marks a payment as confirmed by the payment gateway.
func (s *Store) ConfirmPayment(ctx context.Context, paymentID, paymentHash string) error {
	err := s.database.Update(func(tx db.Transaction) error {
		var payment models.Payment
		if err := tx.GetBucket(db.BucketPayments).Get(paymentID, &payment); err != nil {
			return fmt.Errorf("payment not found: %w", err)
		}

		// Remove old status index
		tx.GetBucket(db.BucketPayments).DeleteIndex(db.BucketPaymentsByStatus, payment.Status+":"+payment.ID)

		now := time.Now()
		payment.Status = "confirmed"
		payment.PaymentHash = &paymentHash
		payment.ConfirmedAt = &now
		payment.UpdatedAt = now

		if err := tx.GetBucket(db.BucketPayments).Put(paymentID, &payment); err != nil {
			return fmt.Errorf("failed to confirm payment: %w", err)
		}

		// Add new status index
		return tx.GetBucket(db.BucketPayments).AddIndex(db.BucketPaymentsByStatus, payment.Status+":"+payment.ID, payment.ID)
	})

	if err == nil {
		metrics.RecordPayment("confirmed")
	}

	return err
}

// FulfillPayment dispatches the payment to the appropriate handler based on item type.
func (s *Store) FulfillPayment(ctx context.Context, paymentID string) error {
	var payment models.Payment
	var item models.Item

	// Load payment and item
	if err := s.database.View(func(tx db.Transaction) error {
		if err := tx.GetBucket(db.BucketPayments).Get(paymentID, &payment); err != nil {
			return fmt.Errorf("payment not found: %w", err)
		}

		if err := tx.GetBucket(db.BucketItems).Get(payment.ItemID, &item); err != nil {
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

	// Execute handler with timing
	start := time.Now()
	result, err := h.Handle(ctx, &payment, &item)
	duration := time.Since(start).Seconds()

	if err != nil {
		metrics.HandlerErrors.WithLabelValues(item.BackendType).Inc()
		// Update payment with failure status
		s.database.Update(func(tx db.Transaction) error {
			tx.GetBucket(db.BucketPayments).DeleteIndex(db.BucketPaymentsByStatus, payment.Status+":"+payment.ID)
			payment.Status = "fulfillment_failed"
			payment.UpdatedAt = time.Now()
			tx.GetBucket(db.BucketPayments).Put(paymentID, &payment)
			tx.GetBucket(db.BucketPayments).AddIndex(db.BucketPaymentsByStatus, payment.Status+":"+payment.ID, payment.ID)
			return nil
		})
		metrics.RecordPayment("fulfillment_failed")
		return fmt.Errorf("handler failed: %w", err)
	}

	metrics.RecordFulfillment(item.BackendType, duration)

	// Update payment with fulfillment result
	return s.database.Update(func(tx db.Transaction) error {
		tx.GetBucket(db.BucketPayments).DeleteIndex(db.BucketPaymentsByStatus, payment.Status+":"+payment.ID)

		now := time.Now()
		payment.Status = "fulfilled"
		payment.FulfillmentResult = models.JSONMap(result)
		payment.FulfilledAt = &now
		payment.UpdatedAt = now

		if err := tx.GetBucket(db.BucketPayments).Put(paymentID, &payment); err != nil {
			return fmt.Errorf("failed to update fulfillment: %w", err)
		}

		return tx.GetBucket(db.BucketPayments).AddIndex(db.BucketPaymentsByStatus, payment.Status+":"+payment.ID, payment.ID)
	})
}

// GetPayment retrieves a single payment by ID.
func (s *Store) GetPayment(ctx context.Context, paymentID string) (*models.Payment, error) {
	var payment models.Payment

	err := s.database.View(func(tx db.Transaction) error {
		return tx.GetBucket(db.BucketPayments).Get(paymentID, &payment)
	})
	if err != nil {
		return nil, fmt.Errorf("payment not found: %w", err)
	}

	return &payment, nil
}

// GetPaymentByInvoiceID retrieves a single payment by invoice ID.
func (s *Store) GetPaymentByInvoiceID(ctx context.Context, invoiceID string) (*models.Payment, error) {
	var payment models.Payment

	err := s.database.View(func(tx db.Transaction) error {
		paymentID, err := tx.GetBucket(db.BucketPayments).GetIndex(db.BucketPaymentsByInvoice, invoiceID)
		if err != nil {
			return err
		}

		return tx.GetBucket(db.BucketPayments).Get(paymentID, &payment)
	})
	if err != nil {
		return nil, fmt.Errorf("payment not found: %w", err)
	}

	return &payment, nil
}

// ListPayments retrieves a list of payments with optional filtering.
func (s *Store) ListPayments(ctx context.Context, filters map[string]interface{}) ([]*models.Payment, error) {
	var payments []*models.Payment

	err := s.database.View(func(tx db.Transaction) error {
		// Get all payments
		var allPayments []*models.Payment
		if err := tx.GetBucket(db.BucketPayments).GetAll(&allPayments); err != nil {
			return fmt.Errorf("failed to load payments: %w", err)
		}

		// Get filter values
		statusFilter, hasStatus := filters["status"].(string)
		itemIDFilter, hasItemID := filters["item_id"].(string)

		// Apply filters
		for _, payment := range allPayments {
			if hasStatus && payment.Status != statusFilter {
				continue
			}
			if hasItemID && payment.ItemID != itemIDFilter {
				continue
			}
			payments = append(payments, payment)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list payments: %w", err)
	}

	return payments, nil
}

// UpdateFulfillmentResult updates the fulfillment result for a payment.
func (s *Store) UpdateFulfillmentResult(ctx context.Context, paymentID string, result models.JSONMap) error {
	return s.database.Update(func(tx db.Transaction) error {
		var payment models.Payment
		if err := tx.GetBucket(db.BucketPayments).Get(paymentID, &payment); err != nil {
			return fmt.Errorf("payment not found: %w", err)
		}

		payment.FulfillmentResult = result
		payment.UpdatedAt = time.Now()

		return tx.GetBucket(db.BucketPayments).Put(paymentID, &payment)
	})
}

// GetCatalog retrieves all categories and items for browsing.
func (s *Store) GetCatalog(ctx context.Context) (map[string]interface{}, error) {
	var categories []*models.Category
	var items []*models.Item

	err := s.database.View(func(tx db.Transaction) error {
		if err := tx.GetBucket(db.BucketCategories).GetAll(&categories); err != nil {
			return fmt.Errorf("failed to load categories: %w", err)
		}

		if err := tx.GetBucket(db.BucketItems).GetAll(&items); err != nil {
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

	err := s.database.View(func(tx db.Transaction) error {
		return tx.GetBucket(db.BucketItems).Get(itemID, &item)
	})
	if err != nil {
		return nil, fmt.Errorf("item not found: %w", err)
	}

	// Decrypt backend config if encryption is enabled
	if s.encryption != nil && len(item.BackendConfig) > 0 {
		decryptedConfig, err := s.decryptBackendConfig(item.BackendConfig)
		if err != nil {
			// If decryption fails, assume it's plaintext (backward compatibility)
			// and return as-is
			return &item, nil
		}
		item.BackendConfig = decryptedConfig
	}

	return &item, nil
}

// SubmitFormData stores form submission data for a payment.
func (s *Store) SubmitFormData(ctx context.Context, paymentID string, formData models.JSONMap) (*models.FormSubmission, error) {
	submission := models.NewFormSubmission(paymentID, formData)

	err := s.database.Update(func(tx db.Transaction) error {
		return tx.GetBucket(db.BucketFormSubmissions).Put(submission.ID, submission)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to save form data: %w", err)
	}

	return submission, nil
}

// GetFormSubmission retrieves form data for a payment.
func (s *Store) GetFormSubmission(ctx context.Context, paymentID string) (*models.FormSubmission, error) {
	var submission *models.FormSubmission

	err := s.database.View(func(tx db.Transaction) error {
		// Get all form submissions
		var allSubmissions []*models.FormSubmission
		if err := tx.GetBucket(db.BucketFormSubmissions).GetAll(&allSubmissions); err != nil {
			return err
		}

		// Find the submission for this payment
		for _, sub := range allSubmissions {
			if sub.PaymentID == paymentID {
				submission = sub
				return nil
			}
		}

		return nil
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

	err := s.database.Update(func(tx db.Transaction) error {
		return tx.GetBucket(db.BucketCategories).Put(category.ID, category)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create category: %w", err)
	}

	return category, nil
}

// ListCategories retrieves all categories ordered by order field.
func (s *Store) ListCategories(ctx context.Context) ([]*models.Category, error) {
	var categories []*models.Category

	err := s.database.View(func(tx db.Transaction) error {
		return tx.GetBucket(db.BucketCategories).GetAll(&categories)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list categories: %w", err)
	}

	return categories, nil
}

// UpdateCategory updates a category by ID.
func (s *Store) UpdateCategory(ctx context.Context, id string, updates map[string]interface{}) error {
	return s.database.Update(func(tx db.Transaction) error {
		var category models.Category
		if err := tx.GetBucket(db.BucketCategories).Get(id, &category); err != nil {
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

		return tx.GetBucket(db.BucketCategories).Put(id, &category)
	})
}

// DeleteCategory deletes a category by ID.
func (s *Store) DeleteCategory(ctx context.Context, id string) error {
	return s.database.Update(func(tx db.Transaction) error {
		return tx.GetBucket(db.BucketCategories).Delete(id)
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

	// Encrypt backend config if encryption is enabled
	itemToStore := *item
	if s.encryption != nil && len(item.BackendConfig) > 0 {
		encryptedConfig, err := s.encryptBackendConfig(item.BackendConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt backend_config: %w", err)
		}
		itemToStore.BackendConfig = encryptedConfig
	}

	err = s.database.Update(func(tx db.Transaction) error {
		if err := tx.GetBucket(db.BucketItems).Put(itemToStore.ID, &itemToStore); err != nil {
			return err
		}

		// Add category index
		if itemToStore.CategoryID != "" {
			return tx.GetBucket(db.BucketPayments).AddIndex(db.BucketItemsByCategory, itemToStore.CategoryID+":"+itemToStore.ID, itemToStore.ID)
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

	err := s.database.View(func(tx db.Transaction) error {
		// Get all items
		var allItems []*models.Item
		if err := tx.GetBucket(db.BucketItems).GetAll(&allItems); err != nil {
			return fmt.Errorf("failed to load items: %w", err)
		}

		// Get filter values
		categoryIDFilter, hasCategoryID := filters["category_id"].(string)
		backendTypeFilter, hasBackendType := filters["backend_type"].(string)
		activeFilter, hasActive := filters["active"].(bool)

		// Apply filters
		for _, item := range allItems {
			if hasCategoryID && item.CategoryID != categoryIDFilter {
				continue
			}
			if hasBackendType && item.BackendType != backendTypeFilter {
				continue
			}
			if hasActive && item.Active != activeFilter {
				continue
			}
			items = append(items, item)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list items: %w", err)
	}

	// Decrypt backend configs if encryption is enabled
	if s.encryption != nil {
		for _, item := range items {
			if len(item.BackendConfig) > 0 {
				decryptedConfig, err := s.decryptBackendConfig(item.BackendConfig)
				if err != nil {
					// If decryption fails, assume it's plaintext (backward compatibility)
					continue
				}
				item.BackendConfig = decryptedConfig
			}
		}
	}

	return items, nil
}

// UpdateItem updates an item by ID.
func (s *Store) UpdateItem(ctx context.Context, id string, updates map[string]interface{}) error {
	// Validate backend changes if present
	if err := s.validateBackendUpdate(ctx, id, updates); err != nil {
		return err
	}

	return s.database.Update(func(tx db.Transaction) error {
		var item models.Item
		if err := tx.GetBucket(db.BucketItems).Get(id, &item); err != nil {
			return fmt.Errorf("item not found: %w", err)
		}

		// Decrypt existing backend config if needed before applying updates
		if s.encryption != nil && len(item.BackendConfig) > 0 {
			decryptedConfig, err := s.decryptBackendConfig(item.BackendConfig)
			if err == nil {
				item.BackendConfig = decryptedConfig
			}
			// If decryption fails, continue with existing config (backward compatibility)
		}

		// Apply updates using helper functions
		applyBasicFieldUpdates(&item, updates)
		applyBackendUpdates(&item, updates)
		if err := applyCategoryUpdate(tx, &item, updates); err != nil {
			return err
		}

		item.UpdatedAt = time.Now()

		// Encrypt backend config if encryption is enabled before storing
		if s.encryption != nil && len(item.BackendConfig) > 0 {
			encryptedConfig, err := s.encryptBackendConfig(item.BackendConfig)
			if err != nil {
				return fmt.Errorf("failed to encrypt backend_config: %w", err)
			}
			item.BackendConfig = encryptedConfig
		}

		return tx.GetBucket(db.BucketItems).Put(id, &item)
	})
}

// applyBasicFieldUpdates applies simple field updates to an item.
func applyBasicFieldUpdates(item *models.Item, updates map[string]interface{}) {
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
	if metadata, ok := updates["metadata"].(models.JSONMap); ok {
		item.Metadata = metadata
	}
	if active, ok := updates["active"].(bool); ok {
		item.Active = active
	}
}

// applyBackendUpdates applies backend-related field updates to an item.
func applyBackendUpdates(item *models.Item, updates map[string]interface{}) {
	if backendType, ok := updates["backend_type"].(string); ok {
		item.BackendType = backendType
	}
	if backendConfig, ok := updates["backend_config"].(models.JSONMap); ok {
		item.BackendConfig = backendConfig
	}
}

// applyCategoryUpdate updates the item's category and manages the category index.
func applyCategoryUpdate(tx db.Transaction, item *models.Item, updates map[string]interface{}) error {
	categoryID, ok := updates["category_id"].(string)
	if !ok {
		return nil
	}

	// Remove old category index
	if item.CategoryID != "" {
		tx.GetBucket(db.BucketPayments).DeleteIndex(db.BucketItemsByCategory, item.CategoryID+":"+item.ID)
	}

	// Update category and add new index
	item.CategoryID = categoryID
	if categoryID != "" {
		tx.GetBucket(db.BucketPayments).AddIndex(db.BucketItemsByCategory, categoryID+":"+item.ID, item.ID)
	}

	return nil
}

// validateBackendUpdate validates backend_type and backend_config changes.
func (s *Store) validateBackendUpdate(ctx context.Context, itemID string, updates map[string]interface{}) error {
	backendType, hasType := updates["backend_type"].(string)
	backendConfig, hasConfig := updates["backend_config"].(models.JSONMap)

	// No backend changes, no validation needed
	if !hasType && !hasConfig {
		return nil
	}

	validationType, err := s.DetermineValidationType(ctx, itemID, backendType, hasType)
	if err != nil {
		return err
	}

	h, err := s.registry.Get(validationType)
	if err != nil {
		return fmt.Errorf("invalid backend_type %q: handler not registered", validationType)
	}

	configToValidate, err := s.DetermineConfigToValidate(ctx, itemID, backendConfig, hasConfig)
	if err != nil {
		return err
	}

	if err := h.Validate(configToValidate); err != nil {
		return fmt.Errorf("invalid backend_config: %w", err)
	}

	return nil
}

// DetermineValidationType returns the backend type to use for validation.
// If hasType is true, it returns the provided backendType. Otherwise, it fetches
// the type from the existing item.
func (s *Store) DetermineValidationType(ctx context.Context, itemID, backendType string, hasType bool) (string, error) {
	if hasType {
		return backendType, nil
	}

	item, err := s.GetItem(ctx, itemID)
	if err != nil {
		return "", fmt.Errorf("item not found: %w", err)
	}

	return item.BackendType, nil
}

// DetermineConfigToValidate returns the backend config to validate.
// If hasConfig is true, it returns the provided backendConfig. Otherwise, it fetches
// the config from the existing item.
func (s *Store) DetermineConfigToValidate(ctx context.Context, itemID string, backendConfig models.JSONMap, hasConfig bool) (models.JSONMap, error) {
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

	err := s.database.Update(func(tx db.Transaction) error {
		return tx.GetBucket(db.BucketTags).Put(tag.ID, tag)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create tag: %w", err)
	}

	return tag, nil
}

// ListTags retrieves all tags.
func (s *Store) ListTags(ctx context.Context) ([]*models.Tag, error) {
	var tags []*models.Tag

	err := s.database.View(func(tx db.Transaction) error {
		return tx.GetBucket(db.BucketTags).GetAll(&tags)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list tags: %w", err)
	}

	return tags, nil
}

// UpdateTag updates a tag by ID.
func (s *Store) UpdateTag(ctx context.Context, id string, updates map[string]interface{}) error {
	return s.database.Update(func(tx db.Transaction) error {
		var tag models.Tag
		if err := tx.GetBucket(db.BucketTags).Get(id, &tag); err != nil {
			return fmt.Errorf("tag not found: %w", err)
		}

		// Apply updates
		if name, ok := updates["name"].(string); ok {
			tag.Name = name
		}
		if slug, ok := updates["slug"].(string); ok {
			tag.Slug = slug
		}

		return tx.GetBucket(db.BucketTags).Put(id, &tag)
	})
}

// DeleteTag deletes a tag by ID.
func (s *Store) DeleteTag(ctx context.Context, id string) error {
	return s.database.Update(func(tx db.Transaction) error {
		// Note: In a real implementation, you'd want to also clean up
		// item-tag associations here
		return tx.GetBucket(db.BucketTags).Delete(id)
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
	return s.database.Update(func(tx db.Transaction) error {
		// Verify item exists
		var item models.Item
		if err := tx.GetBucket(db.BucketItems).Get(itemID, &item); err != nil {
			return fmt.Errorf("item not found: %w", err)
		}

		// Verify tag exists
		var tag models.Tag
		if err := tx.GetBucket(db.BucketTags).Get(tagID, &tag); err != nil {
			return fmt.Errorf("tag not found: %w", err)
		}

		// Modify associations in index buckets
		itemTagKey := itemID + ":" + tagID
		tagItemKey := tagID + ":" + itemID

		if add {
			if err := tx.GetBucket(db.BucketPayments).AddIndex(db.BucketItemTags, itemTagKey, tagID); err != nil {
				return fmt.Errorf("failed to add item-tag association: %w", err)
			}
			if err := tx.GetBucket(db.BucketPayments).AddIndex(db.BucketTagItems, tagItemKey, itemID); err != nil {
				return fmt.Errorf("failed to add tag-item association: %w", err)
			}
		} else {
			if err := tx.GetBucket(db.BucketPayments).DeleteIndex(db.BucketItemTags, itemTagKey); err != nil {
				return fmt.Errorf("failed to remove item-tag association: %w", err)
			}
			if err := tx.GetBucket(db.BucketPayments).DeleteIndex(db.BucketTagItems, tagItemKey); err != nil {
				return fmt.Errorf("failed to remove tag-item association: %w", err)
			}
		}

		return nil
	})
}

// RecordDownload records a download attempt for rate limiting.
func (s *Store) RecordDownload(ctx context.Context, paymentID, ipAddress, userAgent string) error {
	downloadLog := models.NewDownloadLog(paymentID, ipAddress, userAgent)

	return s.database.Update(func(tx db.Transaction) error {
		if err := tx.GetBucket(db.BucketDownloadLogs).Put(downloadLog.ID, downloadLog); err != nil {
			return fmt.Errorf("failed to record download: %w", err)
		}

		// Add payment index
		indexKey := paymentID + ":" + downloadLog.ID
		return tx.GetBucket(db.BucketPayments).AddIndex(db.BucketDownloadsByPayment, indexKey, downloadLog.ID)
	})
}

// GetDownloadCount returns the number of downloads for a payment.
func (s *Store) GetDownloadCount(ctx context.Context, paymentID string) (int, error) {
	count := 0

	err := s.database.View(func(tx db.Transaction) error {
		// Get all download logs
		var allLogs []*models.DownloadLog
		if err := tx.GetBucket(db.BucketDownloadLogs).GetAll(&allLogs); err != nil {
			return err
		}

		// Count logs for this payment
		for _, log := range allLogs {
			if log.PaymentID == paymentID {
				count++
			}
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

// encryptBackendConfig encrypts a backend configuration using the encryption service.
// The config is marshaled to JSON, encrypted, and returned as a JSONMap with an "_encrypted" flag.
func (s *Store) encryptBackendConfig(config models.JSONMap) (models.JSONMap, error) {
	if s.encryption == nil {
		return config, nil
	}

	// Marshal config to JSON
	plaintext, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}

	// Encrypt
	ciphertext, err := s.encryption.Encrypt(plaintext)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt: %w", err)
	}

	// Return as JSONMap with encrypted data and flag
	return models.JSONMap{
		"_encrypted": true,
		"_data":      ciphertext,
	}, nil
}

// decryptBackendConfig decrypts a backend configuration using the encryption service.
// If the config contains the "_encrypted" flag, it decrypts the "_data" field.
// Otherwise, it returns the config as-is (for backward compatibility with plaintext data).
func (s *Store) decryptBackendConfig(config models.JSONMap) (models.JSONMap, error) {
	if s.encryption == nil {
		return config, nil
	}

	// Check if config is encrypted
	encrypted, ok := config["_encrypted"].(bool)
	if !ok || !encrypted {
		// Not encrypted, return as-is (backward compatibility)
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
	plaintext, err := s.encryption.Decrypt(ciphertext)
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

// CreateAuditLog creates a new audit log entry for tracking admin actions.
func (s *Store) CreateAuditLog(ctx context.Context, log *models.AuditLog) error {
	if log == nil {
		return fmt.Errorf("audit log cannot be nil")
	}

	return s.database.Update(func(tx db.Transaction) error {
		bucket := tx.GetBucket(db.BucketAuditLogs)
		return bucket.Put(log.ID, log)
	})
}

// ListAuditLogs retrieves audit logs with optional filtering.
// Supported filters: "action", "resource", "admin_token", "from", "to" (time range).
func (s *Store) ListAuditLogs(ctx context.Context, filters map[string]interface{}) ([]*models.AuditLog, error) {
	var logs []*models.AuditLog

	err := s.database.View(func(tx db.Transaction) error {
		bucket := tx.GetBucket(db.BucketAuditLogs)
		return bucket.GetAll(&logs)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list audit logs: %w", err)
	}

	// Apply filters if provided
	if len(filters) == 0 {
		return logs, nil
	}

	filtered := make([]*models.AuditLog, 0)
	for _, log := range logs {
		if matchesFilters(log, filters) {
			filtered = append(filtered, log)
		}
	}

	return filtered, nil
}

// matchesFilters checks if an audit log matches the given filters.
func matchesFilters(log *models.AuditLog, filters map[string]interface{}) bool {
	if action, ok := filters["action"].(string); ok && log.Action != action {
		return false
	}
	if resource, ok := filters["resource"].(string); ok && log.Resource != resource {
		return false
	}
	if adminToken, ok := filters["admin_token"].(string); ok && log.AdminToken != adminToken {
		return false
	}
	if from, ok := filters["from"].(time.Time); ok && log.Timestamp.Before(from) {
		return false
	}
	if to, ok := filters["to"].(time.Time); ok && log.Timestamp.After(to) {
		return false
	}
	return true
}

// CleanupOldAuditLogs removes audit log entries older than the specified retention period.
// Returns the number of deleted entries.
func (s *Store) CleanupOldAuditLogs(ctx context.Context, retentionDays int) (int, error) {
	if retentionDays <= 0 {
		return 0, fmt.Errorf("retention days must be positive")
	}

	cutoffTime := time.Now().AddDate(0, 0, -retentionDays)
	deletedCount := 0

	err := s.database.Update(func(tx db.Transaction) error {
		// Get all audit logs
		var allLogs []*models.AuditLog
		if err := tx.GetBucket(db.BucketAuditLogs).GetAll(&allLogs); err != nil {
			return fmt.Errorf("failed to list audit logs: %w", err)
		}

		// Delete logs older than cutoff time
		for _, log := range allLogs {
			if log.Timestamp.Before(cutoffTime) {
				if err := tx.GetBucket(db.BucketAuditLogs).Delete(log.ID); err != nil {
					return fmt.Errorf("failed to delete audit log %s: %w", log.ID, err)
				}
				deletedCount++
			}
		}

		return nil
	})
	if err != nil {
		return 0, err
	}

	return deletedCount, nil
}

// Escrow-related methods

// UpdateEscrowState transitions a payment to a new escrow state.
func (s *Store) UpdateEscrowState(ctx context.Context, paymentID string, newState string, additionalData models.JSONMap) error {
	return s.database.Update(func(tx db.Transaction) error {
		var payment models.Payment
		if err := tx.GetBucket(db.BucketPayments).Get(paymentID, &payment); err != nil {
			return fmt.Errorf("payment not found: %w", err)
		}

		if !payment.EscrowEnabled {
			return fmt.Errorf("payment is not an escrow payment")
		}

		// Update escrow state
		payment.EscrowState = newState
		payment.UpdatedAt = time.Now()

		// Merge additional data if provided
		if additionalData != nil {
			if payment.ShippingInfo == nil {
				payment.ShippingInfo = models.JSONMap{}
			}
			for k, v := range additionalData {
				payment.ShippingInfo[k] = v
			}
		}

		// Update payment status based on escrow state
		oldStatus := payment.Status
		switch newState {
		case "released", "refunded":
			payment.Status = "fulfilled"
			now := time.Now()
			payment.FulfilledAt = &now
		}

		// Update status index if status changed
		if oldStatus != payment.Status {
			tx.GetBucket(db.BucketPayments).DeleteIndex(db.BucketPaymentsByStatus, oldStatus+":"+payment.ID)
			tx.GetBucket(db.BucketPayments).AddIndex(db.BucketPaymentsByStatus, payment.Status+":"+payment.ID, payment.ID)
		}

		return tx.GetBucket(db.BucketPayments).Put(paymentID, &payment)
	})
}

// UpdateEscrowSignatures stores signatures for an escrow transaction.
func (s *Store) UpdateEscrowSignatures(ctx context.Context, paymentID string, signatures []models.EscrowSignature) error {
	return s.database.Update(func(tx db.Transaction) error {
		var payment models.Payment
		if err := tx.GetBucket(db.BucketPayments).Get(paymentID, &payment); err != nil {
			return fmt.Errorf("payment not found: %w", err)
		}

		if !payment.EscrowEnabled {
			return fmt.Errorf("payment is not an escrow payment")
		}

		payment.EscrowSignatures = signatures
		payment.UpdatedAt = time.Now()

		return tx.GetBucket(db.BucketPayments).Put(paymentID, &payment)
	})
}

// UpdateEscrowDispute marks a payment as disputed with a reason.
func (s *Store) UpdateEscrowDispute(ctx context.Context, paymentID string, reason string) error {
	return s.database.Update(func(tx db.Transaction) error {
		var payment models.Payment
		if err := tx.GetBucket(db.BucketPayments).Get(paymentID, &payment); err != nil {
			return fmt.Errorf("payment not found: %w", err)
		}

		if !payment.EscrowEnabled {
			return fmt.Errorf("payment is not an escrow payment")
		}

		payment.EscrowState = "disputed"
		payment.DisputeReason = &reason
		payment.UpdatedAt = time.Now()

		return tx.GetBucket(db.BucketPayments).Put(paymentID, &payment)
	})
}

// UpdateEscrowResolution stores the resolution comment for a dispute.
func (s *Store) UpdateEscrowResolution(ctx context.Context, paymentID string, resolution string) error {
	return s.database.Update(func(tx db.Transaction) error {
		var payment models.Payment
		if err := tx.GetBucket(db.BucketPayments).Get(paymentID, &payment); err != nil {
			return fmt.Errorf("payment not found: %w", err)
		}

		if !payment.EscrowEnabled {
			return fmt.Errorf("payment is not an escrow payment")
		}

		payment.DisputeResolution = &resolution
		payment.UpdatedAt = time.Now()

		return tx.GetBucket(db.BucketPayments).Put(paymentID, &payment)
	})
}
