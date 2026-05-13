package test

import (
	"context"
	"testing"
	"time"

	"github.com/opd-ai/store/internal/handlers"
	"github.com/opd-ai/store/pkg/handler"
	"github.com/opd-ai/store/pkg/models"
)

// TestHandlerRegistry tests handler registration and lookup.
func TestHandlerRegistry(t *testing.T) {
	reg := handler.NewRegistry()

	// Register handlers
	handlersToRegister := []handler.FulfillmentHandler{
		handlers.NewDigitalMediaHandler(),
		handlers.NewShippingFormHandler(),
		handlers.NewPrintOnDemandHandler(),
		handlers.NewCustomHandler(),
	}

	for _, h := range handlersToRegister {
		if err := reg.Register(h); err != nil {
			t.Errorf("failed to register handler: %v", err)
		}
	}

	// Verify all handlers are registered
	all := reg.All()
	if len(all) != 4 {
		t.Errorf("expected 4 handlers, got %d", len(all))
	}

	// Test lookup
	digitalHandler, err := reg.Get("digital_media")
	if err != nil {
		t.Errorf("failed to get digital_media handler: %v", err)
	}
	if digitalHandler == nil {
		t.Error("expected handler, got nil")
	}

	// Test lookup of non-existent handler
	_, err = reg.Get("nonexistent")
	if err == nil {
		t.Error("expected error for non-existent handler")
	}

	t.Log("HandlerRegistry test passed")
}

// TestPaymentConfirm tests Payment.Confirm() method.
func TestPaymentConfirm(t *testing.T) {
	payment := models.NewPayment("item-123", "100000", "BTC")

	// Verify initial state
	if payment.Status != "pending" {
		t.Errorf("expected initial status 'pending', got %s", payment.Status)
	}
	if payment.IsConfirmed() {
		t.Error("expected IsConfirmed to return false for pending payment")
	}

	// Confirm payment
	payment.Confirm()

	if payment.Status != "confirmed" {
		t.Errorf("expected status 'confirmed', got %s", payment.Status)
	}
	if !payment.IsConfirmed() {
		t.Error("expected IsConfirmed to return true after confirmation")
	}
	if payment.ConfirmedAt == nil {
		t.Error("expected ConfirmedAt to be set")
	}
}

// TestPaymentFulfill tests Payment.Fulfill() method.
func TestPaymentFulfill(t *testing.T) {
	payment := models.NewPayment("item-123", "100000", "BTC")

	// Confirm payment first
	payment.Confirm()

	// Fulfill payment
	result := map[string]interface{}{
		"download_url": "https://example.com/download",
		"expires_at":   time.Now().Add(24 * time.Hour).Format(time.RFC3339),
	}
	payment.Fulfill(result)

	if payment.Status != "fulfilled" {
		t.Errorf("expected status 'fulfilled', got %s", payment.Status)
	}
	if payment.FulfilledAt == nil {
		t.Error("expected FulfilledAt to be set")
	}
	if len(payment.FulfillmentResult) == 0 {
		t.Error("expected FulfillmentResult to be set")
	}
}

// TestDigitalMediaHandler tests the digital media fulfillment workflow.
func TestDigitalMediaHandler(t *testing.T) {
	h := handlers.NewDigitalMediaHandler()

	// Verify metadata
	meta := h.Metadata()
	if meta.Type != "digital_media" {
		t.Errorf("expected type 'digital_media', got %s", meta.Type)
	}
	if len(meta.RequiredFields) == 0 {
		t.Error("expected required fields, got none")
	}

	// Test validation with valid config
	validConfig := models.JSONMap{
		"storage":          "local",
		"file_path":        "/downloads/product.zip",
		"expiration_hours": 24,
	}
	if err := h.Validate(validConfig); err != nil {
		t.Errorf("validation failed for valid config: %v", err)
	}

	// Test validation with invalid config
	invalidConfig := models.JSONMap{
		"storage":   "invalid",
		"file_path": "/downloads/product.zip",
	}
	if err := h.Validate(invalidConfig); err == nil {
		t.Error("expected validation error for invalid storage type")
	}

	// Test fulfillment
	ctx := context.Background()
	payment := &models.Payment{
		ID:     models.NewID(),
		Status: "confirmed",
	}
	item := &models.Item{
		ID:            models.NewID(),
		BackendConfig: validConfig,
	}

	result, err := h.Handle(ctx, payment, item)
	if err != nil {
		t.Errorf("Handle failed: %v", err)
	}

	// Verify result structure
	if _, ok := result["download_url"]; !ok {
		t.Error("expected download_url in result")
	}
	if _, ok := result["expires_at"]; !ok {
		t.Error("expected expires_at in result")
	}

	t.Log("DigitalMediaHandler test passed")
}

// TestShippingFormHandler tests the shipping form fulfillment workflow.
func TestShippingFormHandler(t *testing.T) {
	h := handlers.NewShippingFormHandler()

	// Verify metadata
	meta := h.Metadata()
	if meta.Type != "shipping_form" {
		t.Errorf("expected type 'shipping_form', got %s", meta.Type)
	}

	// Test validation
	validConfig := models.JSONMap{
		"form_fields": map[string]interface{}{
			"address1": map[string]interface{}{
				"label":    "Street Address",
				"required": true,
			},
		},
	}
	if err := h.Validate(validConfig); err != nil {
		t.Errorf("validation failed: %v", err)
	}

	// Test fulfillment
	ctx := context.Background()
	payment := &models.Payment{
		ID:     models.NewID(),
		Status: "confirmed",
	}
	item := &models.Item{
		ID:            models.NewID(),
		BackendConfig: validConfig,
	}

	result, err := h.Handle(ctx, payment, item)
	if err != nil {
		t.Errorf("Handle failed: %v", err)
	}

	// Verify result
	if _, ok := result["form_url"]; !ok {
		t.Error("expected form_url in result")
	}
	if status, ok := result["status"].(string); !ok || status != "awaiting_address" {
		t.Error("expected status 'awaiting_address'")
	}

	// Test form data validation
	validFormData := handlers.FormData{
		Address1:   "123 Main St",
		City:       "Springfield",
		State:      "IL",
		PostalCode: "62701",
		Country:    "US",
	}
	if err := handlers.ValidateFormData(validFormData); err != nil {
		t.Errorf("form validation failed: %v", err)
	}

	// Test with missing required field
	invalidFormData := handlers.FormData{
		Address1: "123 Main St",
		// City is required but missing
	}
	if err := handlers.ValidateFormData(invalidFormData); err == nil {
		t.Error("expected validation error for missing city")
	}

	t.Log("ShippingFormHandler test passed")
}

// TestPrintOnDemandHandler tests the PoD fulfillment workflow.
func TestPrintOnDemandHandler(t *testing.T) {
	h := handlers.NewPrintOnDemandHandler()

	// Verify metadata
	meta := h.Metadata()
	if meta.Type != "pod" {
		t.Errorf("expected type 'pod', got %s", meta.Type)
	}

	// Test validation with valid config
	validConfig := models.JSONMap{
		"provider":        "redbubble",
		"api_key":         "sk_test_123",
		"product_mapping": map[string]interface{}{},
	}
	if err := h.Validate(validConfig); err != nil {
		t.Errorf("validation failed: %v", err)
	}

	// Test validation with invalid provider
	invalidConfig := models.JSONMap{
		"provider":        "unknown-provider",
		"api_key":         "sk_test_123",
		"product_mapping": map[string]interface{}{},
	}
	if err := h.Validate(invalidConfig); err == nil {
		t.Error("expected validation error for invalid provider")
	}

	// Test fulfillment with non-Printful provider (uses stub response)
	ctx := context.Background()
	payment := &models.Payment{
		ID:     models.NewID(),
		Status: "confirmed",
		PayerInfo: models.JSONMap{
			"name":         "John Doe",
			"address1":     "123 Main St",
			"city":         "New York",
			"state_code":   "NY",
			"country_code": "US",
			"zip":          "10001",
			"email":        "john@example.com",
		},
	}
	item := &models.Item{
		ID: models.NewID(),
	}

	// Add product mapping for this specific item
	item.BackendConfig = models.JSONMap{
		"provider": "redbubble",
		"api_key":  "sk_test_123",
		"product_mapping": map[string]interface{}{
			item.ID: map[string]interface{}{
				"variant_id": "12345",
			},
		},
	}

	result, err := h.Handle(ctx, payment, item)
	if err != nil {
		t.Errorf("Handle failed: %v", err)
	}

	// Verify result
	if _, ok := result["order_id"]; !ok {
		t.Error("expected order_id in result")
	}
	if _, ok := result["tracking_url"]; !ok {
		t.Error("expected tracking_url in result")
	}

	t.Log("PrintOnDemandHandler test passed")
}

// TestCustomHandler tests the custom webhook fulfillment workflow.
func TestCustomHandler(t *testing.T) {
	h := handlers.NewCustomHandler()

	// Verify metadata
	meta := h.Metadata()
	if meta.Type != "custom" {
		t.Errorf("expected type 'custom', got %s", meta.Type)
	}

	// Test validation with valid config
	validConfig := models.JSONMap{
		"webhook_url": "https://example.com/fulfill",
	}
	if err := h.Validate(validConfig); err != nil {
		t.Errorf("validation failed: %v", err)
	}

	// Test validation with invalid URL
	invalidConfig := models.JSONMap{
		"webhook_url": "not-a-valid-url",
	}
	if err := h.Validate(invalidConfig); err == nil {
		t.Error("expected validation error for invalid webhook URL")
	}

	t.Log("CustomHandler test passed")
}

// ===== Table-Driven Tests =====

// TestDigitalMediaHandler_Validate_TableDriven tests digital media handler validation with various configs.
func TestDigitalMediaHandler_Validate_TableDriven(t *testing.T) {
	h := handlers.NewDigitalMediaHandler()

	tests := []struct {
		name      string
		config    models.JSONMap
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid local storage",
			config: models.JSONMap{
				"storage":          "local",
				"file_path":        "/path/to/file.pdf",
				"expiration_hours": 24,
			},
			wantError: false,
		},
		{
			name: "valid s3 storage",
			config: models.JSONMap{
				"storage":          "s3",
				"file_path":        "product.zip",
				"s3_bucket":        "my-bucket",
				"s3_region":        "us-east-1",
				"expiration_hours": 48,
			},
			wantError: false,
		},
		{
			name: "missing storage",
			config: models.JSONMap{
				"file_path":        "/path/to/file.pdf",
				"expiration_hours": 24,
			},
			wantError: false, // Storage defaults to "local"
		},
		{
			name: "invalid storage type",
			config: models.JSONMap{
				"storage":          "ftp",
				"file_path":        "/path/to/file.pdf",
				"expiration_hours": 24,
			},
			wantError: true,
			errorMsg:  "storage",
		},
		{
			name: "missing file_path",
			config: models.JSONMap{
				"storage":          "local",
				"expiration_hours": 24,
			},
			wantError: true,
			errorMsg:  "file_path",
		},
		{
			name: "s3 missing bucket",
			config: models.JSONMap{
				"storage":          "s3",
				"file_path":        "file.pdf",
				"s3_region":        "us-east-1",
				"expiration_hours": 24,
			},
			wantError: true,
			errorMsg:  "s3_bucket",
		},
		{
			name: "s3 missing region",
			config: models.JSONMap{
				"storage":          "s3",
				"file_path":        "file.pdf",
				"s3_bucket":        "my-bucket",
				"expiration_hours": 24,
			},
			wantError: true,
			errorMsg:  "s3_region",
		},
		{
			name: "negative expiration hours",
			config: models.JSONMap{
				"storage":          "local",
				"file_path":        "/path/to/file.pdf",
				"expiration_hours": -1,
			},
			wantError: true,
			errorMsg:  "expiration_hours",
		},
		{
			name: "null config fields",
			config: models.JSONMap{
				"storage":          nil,
				"file_path":        nil,
				"expiration_hours": nil,
			},
			wantError: true,
		},
		{
			name:      "empty config",
			config:    models.JSONMap{},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := h.Validate(tt.config)
			if tt.wantError {
				if err == nil {
					t.Errorf("expected error for %s, got nil", tt.name)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error for %s: %v", tt.name, err)
				}
			}
		})
	}
}

// TestShippingFormHandler_Validate_TableDriven tests shipping form handler validation.
func TestShippingFormHandler_Validate_TableDriven(t *testing.T) {
	h := handlers.NewShippingFormHandler()

	tests := []struct {
		name      string
		config    models.JSONMap
		wantError bool
	}{
		{
			name: "valid config with custom fields",
			config: models.JSONMap{
				"form_fields": map[string]interface{}{
					"name": map[string]interface{}{
						"label":    "Full Name",
						"required": true,
					},
					"address": map[string]interface{}{
						"label":    "Address",
						"required": true,
					},
				},
			},
			wantError: false,
		},
		{
			name:      "empty config (requires form_fields)",
			config:    models.JSONMap{},
			wantError: true,
		},
		{
			name: "config with additional metadata",
			config: models.JSONMap{
				"form_fields": map[string]interface{}{
					"email": map[string]interface{}{
						"label":    "Email",
						"required": true,
						"type":     "email",
					},
				},
				"instructions": "Please provide shipping details",
			},
			wantError: false,
		},
		{
			name: "null form_fields",
			config: models.JSONMap{
				"form_fields": nil,
			},
			wantError: false, // Field exists but is nil - accepted
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := h.Validate(tt.config)
			if tt.wantError {
				if err == nil {
					t.Errorf("expected error for %s, got nil", tt.name)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error for %s: %v", tt.name, err)
				}
			}
		})
	}
}

// TestPrintOnDemandHandler_Validate_TableDriven tests POD handler validation.
func TestPrintOnDemandHandler_Validate_TableDriven(t *testing.T) {
	h := handlers.NewPrintOnDemandHandler()

	tests := []struct {
		name      string
		config    models.JSONMap
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid printful config",
			config: models.JSONMap{
				"provider": "printful",
				"api_key":  "test-api-key",
				"product_mapping": map[string]interface{}{
					"item-123": map[string]interface{}{
						"variant_id": "456",
					},
				},
			},
			wantError: false,
		},
		{
			name: "valid redbubble config",
			config: models.JSONMap{
				"provider": "redbubble",
				"api_key":  "test-api-key",
				"product_mapping": map[string]interface{}{
					"item-123": map[string]interface{}{
						"variant_id": "rb-456",
					},
				},
			},
			wantError: false,
		},
		{
			name: "valid teespring config",
			config: models.JSONMap{
				"provider": "teespring",
				"api_key":  "test-api-key",
				"product_mapping": map[string]interface{}{
					"item-123": map[string]interface{}{
						"variant_id": "ts-456",
					},
				},
			},
			wantError: false,
		},
		{
			name: "missing provider",
			config: models.JSONMap{
				"api_key": "test-api-key",
				"product_mapping": map[string]interface{}{
					"item-123": map[string]interface{}{
						"variant_id": "456",
					},
				},
			},
			wantError: true,
			errorMsg:  "provider",
		},
		{
			name: "missing api_key",
			config: models.JSONMap{
				"provider": "printful",
				"product_mapping": map[string]interface{}{
					"item-123": map[string]interface{}{
						"variant_id": "456",
					},
				},
			},
			wantError: true,
			errorMsg:  "api_key",
		},
		{
			name: "missing product_mapping",
			config: models.JSONMap{
				"provider": "printful",
				"api_key":  "test-api-key",
			},
			wantError: true,
			errorMsg:  "product_mapping",
		},
		{
			name: "invalid provider",
			config: models.JSONMap{
				"provider": "unknown",
				"api_key":  "test-api-key",
				"product_mapping": map[string]interface{}{
					"item-123": map[string]interface{}{
						"variant_id": "456",
					},
				},
			},
			wantError: true,
			errorMsg:  "provider",
		},
		{
			name: "null provider",
			config: models.JSONMap{
				"provider": nil,
				"api_key":  "test-api-key",
				"product_mapping": map[string]interface{}{
					"item-123": map[string]interface{}{
						"variant_id": "456",
					},
				},
			},
			wantError: true,
		},
		{
			name:      "empty config",
			config:    models.JSONMap{},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := h.Validate(tt.config)
			if tt.wantError {
				if err == nil {
					t.Errorf("expected error for %s, got nil", tt.name)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error for %s: %v", tt.name, err)
				}
			}
		})
	}
}

// TestCustomHandler_Validate_TableDriven tests custom handler validation.
func TestCustomHandler_Validate_TableDriven(t *testing.T) {
	h := handlers.NewCustomHandler()

	tests := []struct {
		name      string
		config    models.JSONMap
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid webhook URL",
			config: models.JSONMap{
				"webhook_url": "https://example.com/webhook",
			},
			wantError: false,
		},
		{
			name: "valid webhook with method",
			config: models.JSONMap{
				"webhook_url": "https://api.example.com/fulfill",
				"method":      "POST",
			},
			wantError: false,
		},
		{
			name: "valid webhook with headers",
			config: models.JSONMap{
				"webhook_url": "https://api.example.com/fulfill",
				"headers": map[string]interface{}{
					"Authorization": "Bearer token123",
					"Content-Type":  "application/json",
				},
			},
			wantError: false,
		},
		{
			name: "valid webhook with retry config",
			config: models.JSONMap{
				"webhook_url": "https://api.example.com/fulfill",
				"retry_count": 3,
				"timeout":     30,
			},
			wantError: false,
		},
		{
			name: "missing webhook_url",
			config: models.JSONMap{
				"method": "POST",
			},
			wantError: true,
			errorMsg:  "webhook_url",
		},
		{
			name: "invalid webhook URL format",
			config: models.JSONMap{
				"webhook_url": "not-a-url",
			},
			wantError: true,
			errorMsg:  "webhook_url",
		},
		{
			name: "empty webhook URL",
			config: models.JSONMap{
				"webhook_url": "",
			},
			wantError: true,
			errorMsg:  "webhook_url",
		},
		{
			name: "null webhook_url",
			config: models.JSONMap{
				"webhook_url": nil,
			},
			wantError: true,
		},
		{
			name:      "empty config",
			config:    models.JSONMap{},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := h.Validate(tt.config)
			if tt.wantError {
				if err == nil {
					t.Errorf("expected error for %s, got nil", tt.name)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error for %s: %v", tt.name, err)
				}
			}
		})
	}
}

// TestDigitalMediaHandler_Handle_PaymentStates tests handler with different payment states.
func TestDigitalMediaHandler_Handle_PaymentStates(t *testing.T) {
	h := handlers.NewDigitalMediaHandler()
	ctx := context.Background()

	validConfig := models.JSONMap{
		"storage":          "local",
		"file_path":        "/downloads/test.pdf",
		"expiration_hours": 24,
	}

	tests := []struct {
		name          string
		paymentStatus string
		itemConfig    models.JSONMap
		wantError     bool
		errorMsg      string
		checkResult   func(*testing.T, map[string]interface{})
	}{
		{
			name:          "confirmed payment",
			paymentStatus: "confirmed",
			itemConfig:    validConfig,
			wantError:     false,
			checkResult: func(t *testing.T, result map[string]interface{}) {
				if _, ok := result["download_url"]; !ok {
					t.Error("expected download_url in result")
				}
				if _, ok := result["expires_at"]; !ok {
					t.Error("expected expires_at in result")
				}
			},
		},
		{
			name:          "pending payment - should fail",
			paymentStatus: "pending",
			itemConfig:    validConfig,
			wantError:     true,
			errorMsg:      "payment not confirmed",
		},
		{
			name:          "fulfilled payment - should fail (must be confirmed)",
			paymentStatus: "fulfilled",
			itemConfig:    validConfig,
			wantError:     true,
			errorMsg:      "payment not confirmed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payment := &models.Payment{
				ID:     models.NewID(),
				Status: tt.paymentStatus,
			}
			item := &models.Item{
				ID:            models.NewID(),
				BackendConfig: tt.itemConfig,
			}

			result, err := h.Handle(ctx, payment, item)
			if tt.wantError {
				if err == nil {
					t.Errorf("expected error for %s, got nil", tt.name)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error for %s: %v", tt.name, err)
				}
				if tt.checkResult != nil {
					tt.checkResult(t, result)
				}
			}
		})
	}
}

// TestShippingFormHandler_Handle_PaymentStates tests shipping form handler with payment states.
func TestShippingFormHandler_Handle_PaymentStates(t *testing.T) {
	h := handlers.NewShippingFormHandler()
	ctx := context.Background()

	validConfig := models.JSONMap{
		"form_fields": map[string]interface{}{
			"name": map[string]interface{}{
				"label":    "Name",
				"required": true,
			},
		},
	}

	tests := []struct {
		name          string
		paymentStatus string
		wantError     bool
	}{
		{
			name:          "confirmed payment",
			paymentStatus: "confirmed",
			wantError:     false,
		},
		{
			name:          "fulfilled payment",
			paymentStatus: "fulfilled",
			wantError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payment := &models.Payment{
				ID:     models.NewID(),
				Status: tt.paymentStatus,
			}
			item := &models.Item{
				ID:            models.NewID(),
				BackendConfig: validConfig,
			}

			result, err := h.Handle(ctx, payment, item)
			if tt.wantError {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if result == nil {
					t.Error("expected result, got nil")
				}
			}
		})
	}
}
