package test

import (
	"testing"
	"time"

	"github.com/opd-ai/store/pkg/models"
)

// TestNewPayment tests payment creation.
func TestNewPayment(t *testing.T) {
	payment := models.NewPayment("item-123", "100000", "BTC")

	if payment.ID == "" {
		t.Error("expected payment ID to be set")
	}
	if payment.ItemID != "item-123" {
		t.Errorf("expected ItemID 'item-123', got %s", payment.ItemID)
	}
	if payment.Amount != "100000" {
		t.Errorf("expected Amount '100000', got %s", payment.Amount)
	}
	if payment.Currency != "BTC" {
		t.Errorf("expected Currency 'BTC', got %s", payment.Currency)
	}
	if payment.Status != "pending" {
		t.Errorf("expected Status 'pending', got %s", payment.Status)
	}
	if payment.PayerInfo == nil {
		t.Error("expected PayerInfo to be initialized")
	}
}

// TestPaymentIsConfirmed tests IsConfirmed method.
func TestPaymentIsConfirmed(t *testing.T) {
	payment := models.NewPayment("item-123", "100000", "BTC")

	// Initially should not be confirmed
	if payment.IsConfirmed() {
		t.Error("expected IsConfirmed to return false for new payment")
	}

	// After confirming
	payment.Confirm()
	if !payment.IsConfirmed() {
		t.Error("expected IsConfirmed to return true after Confirm()")
	}
}

// TestPaymentConfirmMethod tests Confirm method.
func TestPaymentConfirmMethod(t *testing.T) {
	payment := models.NewPayment("item-123", "100000", "BTC")

	// Confirm the payment
	payment.Confirm()

	if payment.Status != "confirmed" {
		t.Errorf("expected Status 'confirmed', got %s", payment.Status)
	}
	if payment.ConfirmedAt == nil {
		t.Error("expected ConfirmedAt to be set")
	}
	if time.Since(*payment.ConfirmedAt) > time.Second {
		t.Error("expected ConfirmedAt to be recent")
	}
}

// TestPaymentFulfillMethod tests Fulfill method.
func TestPaymentFulfillMethod(t *testing.T) {
	payment := models.NewPayment("item-123", "100000", "BTC")
	payment.Confirm()

	// Create fulfillment result
	result := models.JSONMap{
		"download_url": "https://example.com/download",
		"expires_at":   time.Now().Add(24 * time.Hour).Format(time.RFC3339),
	}

	payment.Fulfill(result)

	if payment.Status != "fulfilled" {
		t.Errorf("expected Status 'fulfilled', got %s", payment.Status)
	}
	if payment.FulfilledAt == nil {
		t.Error("expected FulfilledAt to be set")
	}
	if time.Since(*payment.FulfilledAt) > time.Second {
		t.Error("expected FulfilledAt to be recent")
	}
	if len(payment.FulfillmentResult) == 0 {
		t.Error("expected FulfillmentResult to contain data")
	}
	if payment.FulfillmentResult["download_url"] != result["download_url"] {
		t.Error("expected FulfillmentResult to match provided result")
	}
}

// TestNewCategory tests category creation.
func TestNewCategory(t *testing.T) {
	cat := models.NewCategory("Electronics", "Electronic products")

	if cat.ID == "" {
		t.Error("expected category ID to be set")
	}
	if cat.Name != "Electronics" {
		t.Errorf("expected Name 'Electronics', got %s", cat.Name)
	}
	if cat.Description != "Electronic products" {
		t.Errorf("expected Description 'Electronic products', got %s", cat.Description)
	}
	if cat.Metadata == nil {
		t.Error("expected Metadata to be initialized")
	}
}

// TestNewTag tests tag creation.
func TestNewTag(t *testing.T) {
	tag := models.NewTag("featured")

	if tag.ID == "" {
		t.Error("expected tag ID to be set")
	}
	if tag.Name != "featured" {
		t.Errorf("expected Name 'featured', got %s", tag.Name)
	}
}

// TestNewItem tests item creation.
func TestNewItem(t *testing.T) {
	item := models.NewItem("cat-123", "Product", "Description", "100000", "BTC", "digital_media")

	if item.ID == "" {
		t.Error("expected item ID to be set")
	}
	if item.CategoryID != "cat-123" {
		t.Errorf("expected CategoryID 'cat-123', got %s", item.CategoryID)
	}
	if item.Name != "Product" {
		t.Errorf("expected Name 'Product', got %s", item.Name)
	}
	if item.Price != "100000" {
		t.Errorf("expected Price '100000', got %s", item.Price)
	}
	if item.Currency != "BTC" {
		t.Errorf("expected Currency 'BTC', got %s", item.Currency)
	}
	if item.BackendType != "digital_media" {
		t.Errorf("expected BackendType 'digital_media', got %s", item.BackendType)
	}
	if !item.Active {
		t.Error("expected item to be active by default")
	}
	if item.BackendConfig == nil {
		t.Error("expected BackendConfig to be initialized")
	}
}

// TestJSONMapMarshalUnmarshal tests JSONMap JSON marshaling and unmarshaling.
func TestJSONMapMarshalUnmarshal(t *testing.T) {
	// Test MarshalJSON
	original := models.JSONMap{
		"key1": "value1",
		"key2": float64(123),
		"key3": true,
	}

	bytes, err := original.MarshalJSON()
	if err != nil {
		t.Errorf("MarshalJSON() error: %v", err)
	}

	if len(bytes) == 0 {
		t.Error("MarshalJSON() should return non-empty []byte")
	}

	// Test UnmarshalJSON
	var scanned models.JSONMap
	err = scanned.UnmarshalJSON(bytes)
	if err != nil {
		t.Errorf("UnmarshalJSON() error: %v", err)
	}

	if scanned["key1"] != "value1" {
		t.Errorf("expected key1='value1', got %v", scanned["key1"])
	}
	if scanned["key2"] != float64(123) {
		t.Errorf("expected key2=123, got %v", scanned["key2"])
	}
	if scanned["key3"] != true {
		t.Errorf("expected key3=true, got %v", scanned["key3"])
	}
}

// TestNewID tests UUID generation.
func TestNewID(t *testing.T) {
	id1 := models.NewID()
	id2 := models.NewID()

	if id1 == "" {
		t.Error("expected non-empty ID")
	}
	if id2 == "" {
		t.Error("expected non-empty ID")
	}
	if id1 == id2 {
		t.Error("expected different IDs for multiple calls")
	}
	if len(id1) != 36 {
		t.Errorf("expected UUID format (36 chars), got %d", len(id1))
	}
}

// TestPaymentLifecycle tests full payment state transitions.
func TestPaymentLifecycle(t *testing.T) {
	payment := models.NewPayment("item-123", "100000", "BTC")

	// Initial state
	if payment.Status != "pending" {
		t.Errorf("expected initial status 'pending', got %s", payment.Status)
	}
	if payment.IsConfirmed() {
		t.Error("payment should not be confirmed initially")
	}

	// Confirm
	payment.Confirm()
	if payment.Status != "confirmed" {
		t.Errorf("expected status 'confirmed' after Confirm(), got %s", payment.Status)
	}
	if !payment.IsConfirmed() {
		t.Error("payment should be confirmed after Confirm()")
	}
	if payment.ConfirmedAt == nil {
		t.Error("ConfirmedAt should be set after Confirm()")
	}

	// Fulfill
	result := models.JSONMap{"status": "success"}
	payment.Fulfill(result)
	if payment.Status != "fulfilled" {
		t.Errorf("expected status 'fulfilled' after Fulfill(), got %s", payment.Status)
	}
	if payment.FulfilledAt == nil {
		t.Error("FulfilledAt should be set after Fulfill()")
	}
	if len(payment.FulfillmentResult) == 0 {
		t.Error("FulfillmentResult should be populated after Fulfill()")
	}
}
