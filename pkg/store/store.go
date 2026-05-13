package store

import (
	"context"
	"fmt"

	"github.com/opd-ai/store/pkg/handler"
	"github.com/opd-ai/store/pkg/models"
	"gorm.io/gorm"
)

// Store orchestrates the payment-to-fulfillment workflow.
// It manages payment creation, confirmation, and handler dispatch.
type Store struct {
	db       *gorm.DB
	registry *handler.Registry
}

// NewStore creates a new Store instance.
func NewStore(db *gorm.DB, registry *handler.Registry) *Store {
	return &Store{
		db:       db,
		registry: registry,
	}
}

// CreatePayment initializes a new payment record.
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - itemID: the item being purchased
//   - amount: the payment amount (as string to preserve precision)
//   - currency: the payment currency (e.g., "BTC", "XMR")
//
// Returns:
//   - *models.Payment: the created payment record
//   - error: database error if creation fails
func (s *Store) CreatePayment(ctx context.Context, itemID, amount, currency string) (*models.Payment, error) {
	payment := models.NewPayment(itemID, amount, currency)

	if err := s.db.WithContext(ctx).Create(payment).Error; err != nil {
		return nil, fmt.Errorf("failed to create payment: %w", err)
	}

	return payment, nil
}

// UpdatePaymentInvoice updates a payment with the invoice ID from the paywall service.
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - paymentID: the payment to update
//   - invoiceID: the invoice ID from the paywall service
//
// Returns:
//   - error: if payment not found or update fails
func (s *Store) UpdatePaymentInvoice(ctx context.Context, paymentID, invoiceID string) error {
	return s.updatePaymentField(ctx, paymentID, "invoice_id", invoiceID, "failed to update payment invoice")
}

// UpdatePaymentPayerInfo updates a payment with payer information.
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - paymentID: the payment to update
//   - payerInfo: the payer information to store
//
// Returns:
//   - error: if payment not found or update fails
func (s *Store) UpdatePaymentPayerInfo(ctx context.Context, paymentID string, payerInfo models.JSONMap) error {
	return s.updatePaymentField(ctx, paymentID, "payer_info", payerInfo, "failed to update payer info")
}

// updatePaymentField updates a single field on a payment record.
func (s *Store) updatePaymentField(ctx context.Context, paymentID string, field string, value interface{}, errorMsg string) error {
	result := s.db.WithContext(ctx).Model(&models.Payment{}).
		Where("id = ?", paymentID).
		Update(field, value)

	if result.Error != nil {
		return fmt.Errorf("%s: %w", errorMsg, result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("payment not found")
	}

	return nil
}

// ConfirmPayment marks a payment as confirmed by the payment gateway.
// This should only be called after verifying the payment with opd-ai/paywall.
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - paymentID: the payment to confirm
//   - paymentHash: the blockchain transaction hash or verification token
//
// Returns:
//   - error: if payment not found or confirmation fails
func (s *Store) ConfirmPayment(ctx context.Context, paymentID, paymentHash string) error {
	payment := &models.Payment{}

	if err := s.db.WithContext(ctx).Model(payment).
		Where("id = ?", paymentID).
		Updates(map[string]interface{}{
			"status":       "confirmed",
			"payment_hash": paymentHash,
			"confirmed_at": gorm.Expr("CURRENT_TIMESTAMP"),
		}).Error; err != nil {
		return fmt.Errorf("failed to confirm payment: %w", err)
	}

	return nil
}

// FulfillPayment dispatches the payment to the appropriate handler based on item type.
// This should be called after ConfirmPayment to execute backend-specific fulfillment.
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - paymentID: the confirmed payment to fulfill
//
// Returns:
//   - error: if payment not found, handler not registered, or handler fails
func (s *Store) FulfillPayment(ctx context.Context, paymentID string) error {
	payment := &models.Payment{}

	// Load payment with item
	if err := s.db.WithContext(ctx).Preload("Item").
		Where("id = ?", paymentID).
		First(payment).Error; err != nil {
		return fmt.Errorf("payment not found: %w", err)
	}

	// Check if payment is confirmed
	if payment.Status != "confirmed" {
		return fmt.Errorf("payment not confirmed (status: %s)", payment.Status)
	}

	// Get the appropriate handler
	h, err := s.registry.Get(payment.Item.BackendType)
	if err != nil {
		return fmt.Errorf("handler for type %q not found: %w", payment.Item.BackendType, err)
	}

	// Execute handler
	result, err := h.Handle(ctx, payment, payment.Item)
	if err != nil {
		// Update payment with failure status
		s.db.WithContext(ctx).Model(payment).Updates(map[string]interface{}{
			"status": "fulfillment_failed",
		})
		return fmt.Errorf("handler failed: %w", err)
	}

	// Convert result to JSONMap
	fulfillmentResult := models.JSONMap(result)

	// Update payment with fulfillment result
	if err := s.db.WithContext(ctx).Model(payment).Updates(map[string]interface{}{
		"status":             "fulfilled",
		"fulfillment_result": fulfillmentResult,
		"fulfilled_at":       gorm.Expr("CURRENT_TIMESTAMP"),
	}).Error; err != nil {
		return fmt.Errorf("failed to update fulfillment: %w", err)
	}

	return nil
}

// GetPayment retrieves a single payment by ID.
func (s *Store) GetPayment(ctx context.Context, paymentID string) (*models.Payment, error) {
	payment := &models.Payment{}

	if err := s.db.WithContext(ctx).Where("id = ?", paymentID).
		First(payment).Error; err != nil {
		return nil, fmt.Errorf("payment not found: %w", err)
	}

	return payment, nil
}

// GetPaymentByInvoiceID retrieves a single payment by invoice ID.
func (s *Store) GetPaymentByInvoiceID(ctx context.Context, invoiceID string) (*models.Payment, error) {
	payment := &models.Payment{}

	if err := s.db.WithContext(ctx).Where("invoice_id = ?", invoiceID).
		First(payment).Error; err != nil {
		return nil, fmt.Errorf("payment not found: %w", err)
	}

	return payment, nil
}

// ListPayments retrieves a list of payments with optional filtering.
// Parameters:
//   - ctx: context for cancellation
//   - filters: a map containing optional filters like "status" and "item_id"
func (s *Store) ListPayments(ctx context.Context, filters map[string]interface{}) ([]*models.Payment, error) {
	var payments []*models.Payment

	query := s.db.WithContext(ctx)

	// Apply status filter if provided
	if status, ok := filters["status"].(string); ok && status != "" {
		query = query.Where("status = ?", status)
	}

	// Apply item_id filter if provided
	if itemID, ok := filters["item_id"].(string); ok && itemID != "" {
		query = query.Where("item_id = ?", itemID)
	}

	if err := query.Order("created_at DESC").Find(&payments).Error; err != nil {
		return nil, fmt.Errorf("failed to list payments: %w", err)
	}

	return payments, nil
}

// UpdateFulfillmentResult updates the fulfillment result for a payment.
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - paymentID: the payment to update
//   - result: the updated fulfillment result data
//
// Returns:
//   - error: if payment not found or update fails
func (s *Store) UpdateFulfillmentResult(ctx context.Context, paymentID string, result models.JSONMap) error {
	payment := &models.Payment{}

	if err := s.db.WithContext(ctx).Model(payment).
		Where("id = ?", paymentID).
		Update("fulfillment_result", result).Error; err != nil {
		return fmt.Errorf("failed to update fulfillment result: %w", err)
	}

	return nil
}

// GetCatalog retrieves all categories and items for browsing.
func (s *Store) GetCatalog(ctx context.Context) (map[string]interface{}, error) {
	var categories []*models.Category
	var items []*models.Item

	if err := s.db.WithContext(ctx).Find(&categories).Error; err != nil {
		return nil, fmt.Errorf("failed to load categories: %w", err)
	}

	if err := s.db.WithContext(ctx).Preload("Category").Preload("Tags").Find(&items).Error; err != nil {
		return nil, fmt.Errorf("failed to load items: %w", err)
	}

	return map[string]interface{}{
		"categories": categories,
		"items":      items,
	}, nil
}

// GetItem retrieves a single item by ID.
func (s *Store) GetItem(ctx context.Context, itemID string) (*models.Item, error) {
	item := &models.Item{}

	if err := s.db.WithContext(ctx).Preload("Tags").
		Where("id = ?", itemID).
		First(item).Error; err != nil {
		return nil, fmt.Errorf("item not found: %w", err)
	}

	return item, nil
}

// SubmitFormData stores form submission data for a payment.
// This is used by handlers like ShippingForm that need to collect user data after payment.
func (s *Store) SubmitFormData(ctx context.Context, paymentID string, formData models.JSONMap) (*models.FormSubmission, error) {
	submission := &models.FormSubmission{
		PaymentID: paymentID,
		FormData:  formData,
	}

	if err := s.db.WithContext(ctx).Create(submission).Error; err != nil {
		return nil, fmt.Errorf("failed to save form data: %w", err)
	}

	return submission, nil
}

// GetFormSubmission retrieves form data for a payment.
func (s *Store) GetFormSubmission(ctx context.Context, paymentID string) (*models.FormSubmission, error) {
	submission := &models.FormSubmission{}

	if err := s.db.WithContext(ctx).Where("payment_id = ?", paymentID).
		First(submission).Error; err != nil {
		return nil, fmt.Errorf("form submission not found: %w", err)
	}

	return submission, nil
}

// HandlerMetadata returns metadata for all registered handlers.
// This is used by the admin API to display available handler types.
func (s *Store) HandlerMetadata() map[string]handler.HandlerMetadata {
	return s.registry.All()
}

// CreateCategory creates a new category and persists it to the database.
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - name: the category name
//   - description: the category description
//
// Returns:
//   - *models.Category: the created category record
//   - error: database error if creation fails
func (s *Store) CreateCategory(ctx context.Context, name, description string) (*models.Category, error) {
	category := models.NewCategory(name, description)

	if err := s.db.WithContext(ctx).Create(category).Error; err != nil {
		return nil, fmt.Errorf("failed to create category: %w", err)
	}

	return category, nil
}

// ListCategories retrieves all categories ordered by order field.
// Parameters:
//   - ctx: context for cancellation and timeouts
//
// Returns:
//   - []*models.Category: list of all categories
//   - error: database error if query fails
func (s *Store) ListCategories(ctx context.Context) ([]*models.Category, error) {
	var categories []*models.Category

	if err := s.db.WithContext(ctx).Order("`order`, created_at ASC").Find(&categories).Error; err != nil {
		return nil, fmt.Errorf("failed to list categories: %w", err)
	}

	return categories, nil
}

// UpdateCategory updates a category by ID.
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - id: the category ID to update
//   - updates: map of fields to update
//
// Returns:
//   - error: if category not found or update fails
func (s *Store) UpdateCategory(ctx context.Context, id string, updates map[string]interface{}) error {
	result := s.db.WithContext(ctx).Model(&models.Category{}).
		Where("id = ?", id).
		Updates(updates)

	if result.Error != nil {
		return fmt.Errorf("failed to update category: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("category not found")
	}

	return nil
}

// DeleteCategory deletes a category by ID.
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - id: the category ID to delete
//
// Returns:
//   - error: if category not found or deletion fails
func (s *Store) DeleteCategory(ctx context.Context, id string) error {
	result := s.db.WithContext(ctx).Delete(&models.Category{}, "id = ?", id)

	if result.Error != nil {
		return fmt.Errorf("failed to delete category: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("category not found")
	}

	return nil
}

// CreateItem creates a new item and persists it to the database.
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - item: the item to create
//
// Returns:
//   - *models.Item: the created item record
//   - error: database error or validation error if creation fails
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

	if err := s.db.WithContext(ctx).Create(item).Error; err != nil {
		return nil, fmt.Errorf("failed to create item: %w", err)
	}

	return item, nil
}

// ListItems retrieves items with optional filtering.
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - filters: optional filters (category_id, backend_type, active)
//
// Returns:
//   - []*models.Item: list of items matching filters
//   - error: database error if query fails
func (s *Store) ListItems(ctx context.Context, filters map[string]interface{}) ([]*models.Item, error) {
	var items []*models.Item

	query := s.db.WithContext(ctx).Preload("Category").Preload("Tags")

	// Apply category_id filter if provided
	if categoryID, ok := filters["category_id"].(string); ok && categoryID != "" {
		query = query.Where("category_id = ?", categoryID)
	}

	// Apply backend_type filter if provided
	if backendType, ok := filters["backend_type"].(string); ok && backendType != "" {
		query = query.Where("backend_type = ?", backendType)
	}

	// Apply active filter if provided
	if active, ok := filters["active"].(bool); ok {
		query = query.Where("active = ?", active)
	}

	if err := query.Order("created_at DESC").Find(&items).Error; err != nil {
		return nil, fmt.Errorf("failed to list items: %w", err)
	}

	return items, nil
}

// UpdateItem updates an item by ID.
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - id: the item ID to update
//   - updates: map of fields to update
//
// Returns:
//   - error: if item not found or update fails
func (s *Store) UpdateItem(ctx context.Context, id string, updates map[string]interface{}) error {
	// Validate backend changes if present
	if err := s.validateBackendUpdate(ctx, id, updates); err != nil {
		return err
	}

	result := s.db.WithContext(ctx).Model(&models.Item{}).
		Where("id = ?", id).
		Updates(updates)

	if result.Error != nil {
		return fmt.Errorf("failed to update item: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("item not found")
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
// If a new type is provided, it uses that; otherwise it fetches the existing item's type.
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
// If a new config is provided, it uses that; otherwise it fetches the existing item's config.
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
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - id: the item ID to delete
//
// Returns:
//   - error: if item not found or deletion fails
func (s *Store) DeleteItem(ctx context.Context, id string) error {
	result := s.db.WithContext(ctx).Model(&models.Item{}).
		Where("id = ?", id).
		Update("active", false)

	if result.Error != nil {
		return fmt.Errorf("failed to delete item: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("item not found")
	}

	return nil
}

// CreateTag creates a new tag and persists it to the database.
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - name: the tag name
//
// Returns:
//   - *models.Tag: the created tag record
//   - error: database error if creation fails
func (s *Store) CreateTag(ctx context.Context, name string) (*models.Tag, error) {
	tag := models.NewTag(name)

	if err := s.db.WithContext(ctx).Create(tag).Error; err != nil {
		return nil, fmt.Errorf("failed to create tag: %w", err)
	}

	return tag, nil
}

// ListTags retrieves all tags.
// Parameters:
//   - ctx: context for cancellation and timeouts
//
// Returns:
//   - []*models.Tag: list of all tags
//   - error: database error if query fails
func (s *Store) ListTags(ctx context.Context) ([]*models.Tag, error) {
	var tags []*models.Tag

	if err := s.db.WithContext(ctx).Order("name ASC").Find(&tags).Error; err != nil {
		return nil, fmt.Errorf("failed to list tags: %w", err)
	}

	return tags, nil
}

// UpdateTag updates a tag by ID.
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - id: the tag ID to update
//   - updates: map of fields to update
//
// Returns:
//   - error: if tag not found or update fails
func (s *Store) UpdateTag(ctx context.Context, id string, updates map[string]interface{}) error {
	result := s.db.WithContext(ctx).Model(&models.Tag{}).
		Where("id = ?", id).
		Updates(updates)

	if result.Error != nil {
		return fmt.Errorf("failed to update tag: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("tag not found")
	}

	return nil
}

// DeleteTag deletes a tag by ID.
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - id: the tag ID to delete
//
// Returns:
//   - error: if tag not found or deletion fails
func (s *Store) DeleteTag(ctx context.Context, id string) error {
	result := s.db.WithContext(ctx).Delete(&models.Tag{}, "id = ?", id)

	if result.Error != nil {
		return fmt.Errorf("failed to delete tag: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("tag not found")
	}

	return nil
}

// AddItemTag associates a tag with an item.
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - itemID: the item ID
//   - tagID: the tag ID
//
// Returns:
//   - error: if item or tag not found, or association fails
func (s *Store) AddItemTag(ctx context.Context, itemID, tagID string) error {
	return s.modifyItemTagAssociation(ctx, itemID, tagID, true)
}

// RemoveItemTag removes a tag association from an item.
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - itemID: the item ID
//   - tagID: the tag ID
//
// Returns:
//   - error: if item or tag not found, or removal fails
func (s *Store) RemoveItemTag(ctx context.Context, itemID, tagID string) error {
	return s.modifyItemTagAssociation(ctx, itemID, tagID, false)
}

// modifyItemTagAssociation handles adding or removing tag associations.
func (s *Store) modifyItemTagAssociation(ctx context.Context, itemID, tagID string, add bool) error {
	// Verify item exists
	item := &models.Item{}
	if err := s.db.WithContext(ctx).Where("id = ?", itemID).First(item).Error; err != nil {
		return fmt.Errorf("item not found: %w", err)
	}

	// Verify tag exists
	tag := &models.Tag{}
	if err := s.db.WithContext(ctx).Where("id = ?", tagID).First(tag).Error; err != nil {
		return fmt.Errorf("tag not found: %w", err)
	}

	// Modify association
	assoc := s.db.WithContext(ctx).Model(item).Association("Tags")
	var err error
	if add {
		err = assoc.Append(tag)
	} else {
		err = assoc.Delete(tag)
	}

	if err != nil {
		action := "add"
		if !add {
			action = "remove"
		}
		return fmt.Errorf("failed to %s tag to item: %w", action, err)
	}

	return nil
}

// RecordDownload records a download attempt for rate limiting.
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - paymentID: the payment ID
//   - ipAddress: the downloader's IP address
//   - userAgent: the downloader's user agent
//
// Returns:
//   - error: if recording fails
func (s *Store) RecordDownload(ctx context.Context, paymentID, ipAddress, userAgent string) error {
	downloadLog := models.NewDownloadLog(paymentID, ipAddress, userAgent)

	if err := s.db.WithContext(ctx).Create(downloadLog).Error; err != nil {
		return fmt.Errorf("failed to record download: %w", err)
	}

	return nil
}

// GetDownloadCount returns the number of downloads for a payment.
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - paymentID: the payment ID
//
// Returns:
//   - int: the download count
//   - error: if query fails
func (s *Store) GetDownloadCount(ctx context.Context, paymentID string) (int, error) {
	var count int64

	if err := s.db.WithContext(ctx).Model(&models.DownloadLog{}).
		Where("payment_id = ?", paymentID).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("failed to get download count: %w", err)
	}

	return int(count), nil
}

// CheckDownloadLimit checks if a payment has exceeded its download limit.
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - paymentID: the payment ID
//   - maxDownloads: the maximum allowed downloads (0 means unlimited)
//
// Returns:
//   - bool: true if limit is exceeded
//   - error: if check fails
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
