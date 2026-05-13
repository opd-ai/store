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
		"provider":        "printful",
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
