package test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/opd-ai/store/internal/api"
	"github.com/opd-ai/store/pkg/handler"
	"github.com/opd-ai/store/pkg/models"
	"github.com/opd-ai/store/pkg/paywall"
	"github.com/opd-ai/store/pkg/store"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupIntegrationTest creates test infrastructure for integration tests
func setupIntegrationTest(t *testing.T) (*api.Handler, *gorm.DB, *store.Store, *mux.Router) {
	// Create in-memory database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	// Run migrations
	err = db.AutoMigrate(
		&models.Category{},
		&models.Item{},
		&models.Tag{},
		&models.Payment{},
		&models.FormSubmission{},
		&models.DownloadLog{},
	)
	if err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	// Create handler registry with mock handler
	reg := handler.NewRegistry()
	mockHandler := &testIntegrationHandler{
		handlerType: "mock",
	}
	if err := reg.Register(mockHandler); err != nil {
		t.Fatalf("Failed to register handler: %v", err)
	}

	// Create store
	s := store.NewStore(db, reg)

	// Create mock paywall client (for integration tests, we still mock external services)
	paywallClient := paywall.NewClient("http://test-paywall", "test-api-key")

	h := api.NewHandler(s, paywallClient)

	// Create router and register routes
	r := mux.NewRouter()

	// Public routes
	r.HandleFunc("/health", api.HealthHandler).Methods("GET")
	r.HandleFunc("/api/catalog", h.GetCatalog).Methods("GET")
	r.HandleFunc("/api/item/{id}", h.GetItem).Methods("GET")
	r.HandleFunc("/api/checkout", h.CreateCheckout).Methods("POST")
	r.HandleFunc("/api/payment/{id}/status", h.GetPaymentStatus).Methods("GET")
	r.HandleFunc("/api/payment/{id}/submit-form", h.SubmitPaymentForm).Methods("POST")
	r.HandleFunc("/api/payment/{id}/track-download", h.TrackDownload).Methods("POST")

	// Admin routes (wrapped in middleware but we'll set token in requests)
	r.HandleFunc("/admin/payments", h.ListPayments).Methods("GET")
	r.HandleFunc("/admin/payments/{id}/confirm", h.ConfirmPayment).Methods("POST")
	r.HandleFunc("/admin/payments/{id}/fulfill", h.FulfillPayment).Methods("POST")
	r.HandleFunc("/admin/categories", h.CreateCategory).Methods("POST")
	r.HandleFunc("/admin/categories", h.ListCategories).Methods("GET")
	r.HandleFunc("/admin/items", h.CreateItem).Methods("POST")
	r.HandleFunc("/admin/items", h.ListItems).Methods("GET")

	return h, db, s, r
}

// For methods not directly available in store, use GORM db.Create()
func createCategory(db *gorm.DB, name, description string) *models.Category {
	cat := models.NewCategory(name, description)
	db.Create(cat)
	return cat
}

func createItem(db *gorm.DB, categoryID, name, description, price, currency, backendType string, config models.JSONMap) *models.Item {
	item := models.NewItem(categoryID, name, description, price, currency, backendType)
	item.BackendConfig = config
	db.Create(item)
	return item
}

func createPayment(db *gorm.DB, itemID, amount, currency string) *models.Payment {
	payment := models.NewPayment(itemID, amount, currency)
	db.Create(payment)
	return payment
}

// testIntegrationHandler implements a mock fulfillment handler for integration tests
type testIntegrationHandler struct {
	handlerType string
}

func (th *testIntegrationHandler) Metadata() handler.HandlerMetadata {
	return handler.HandlerMetadata{
		Type:        th.handlerType,
		DisplayName: "Mock Handler",
		Description: "Test handler for integration tests",
	}
}

func (th *testIntegrationHandler) Validate(config models.JSONMap) error {
	return nil
}

func (th *testIntegrationHandler) RequiresForm() bool {
	return false
}

func (th *testIntegrationHandler) GetFormSchema() map[string]interface{} {
	return nil
}

func (th *testIntegrationHandler) Handle(ctx context.Context, payment *models.Payment, item *models.Item) (map[string]interface{}, error) {
	return map[string]interface{}{
		"status":       "fulfilled",
		"tracking_url": "http://example.com/track/123",
		"message":      "Mock fulfillment successful",
	}, nil
}

// TestPaymentFlow_EndToEnd tests the complete payment lifecycle
func TestPaymentFlow_EndToEnd(t *testing.T) {
	os.Setenv("STORE_ADMIN_TOKEN", "test-admin-token")
	defer os.Unsetenv("STORE_ADMIN_TOKEN")

	_, db, s, r := setupIntegrationTest(t)
	ctx := context.Background()

	// Step 1: Create a category
	category := createCategory(db, "Electronics", "Electronic items")

	// Step 2: Create an item
	item := createItem(db, category.ID, "Laptop", "High-end laptop", "1000.00", "BTC", "mock", models.JSONMap{
		"test_field": "value",
	})

	// Step 3: Verify catalog listing
	catalogReq := httptest.NewRequest(http.MethodGet, "/api/catalog", nil)
	catalogW := httptest.NewRecorder()
	r.ServeHTTP(catalogW, catalogReq)

	if catalogW.Code != http.StatusOK {
		t.Errorf("Catalog request failed: %d", catalogW.Code)
	}

	var catalogResp struct {
		Categories []models.Category `json:"categories"`
	}
	json.NewDecoder(catalogW.Body).Decode(&catalogResp)
	if len(catalogResp.Categories) == 0 {
		t.Error("Expected at least one category in catalog")
	}

	// Step 4: Get item details
	itemReq := httptest.NewRequest(http.MethodGet, "/api/item/"+item.ID, nil)
	itemW := httptest.NewRecorder()
	r.ServeHTTP(itemW, itemReq)

	if itemW.Code != http.StatusOK {
		t.Errorf("Get item request failed: %d", itemW.Code)
	}

	// Step 5: Create payment (simulate checkout)
	payment := createPayment(db, item.ID, "1000.00", "BTC")

	if payment.Status != "pending" {
		t.Errorf("Expected payment status pending, got %s", payment.Status)
	}

	// Step 6: Check payment status (should be pending)
	statusReq := httptest.NewRequest(http.MethodGet, "/api/payment/"+payment.ID+"/status", nil)
	statusW := httptest.NewRecorder()
	r.ServeHTTP(statusW, statusReq)

	if statusW.Code != http.StatusOK {
		t.Errorf("Payment status request failed: %d", statusW.Code)
	}

	var statusResp struct {
		Status string `json:"status"`
	}
	json.NewDecoder(statusW.Body).Decode(&statusResp)
	if statusResp.Status != "pending" {
		t.Errorf("Expected pending status, got %s", statusResp.Status)
	}

	// Step 7: Admin confirms payment
	confirmBody := map[string]string{"payment_hash": "test_hash_123"}
	confirmJSON, _ := json.Marshal(confirmBody)
	confirmReq := httptest.NewRequest(http.MethodPost, "/admin/payments/"+payment.ID+"/confirm", bytes.NewReader(confirmJSON))
	confirmReq.Header.Set("X-Admin-Token", "test-admin-token")
	confirmW := httptest.NewRecorder()
	r.ServeHTTP(confirmW, confirmReq)

	if confirmW.Code != http.StatusOK {
		t.Errorf("Confirm payment failed: %d - %s", confirmW.Code, confirmW.Body.String())
	}

	// Step 8: Verify payment is confirmed
	var err error
	payment, err = s.GetPayment(ctx, payment.ID)
	if err != nil {
		t.Fatalf("Failed to retrieve payment: %v", err)
	}
	if payment.Status != "confirmed" {
		t.Errorf("Expected payment status confirmed, got %s", payment.Status)
	}
	if payment.PaymentHash == nil || *payment.PaymentHash != "test_hash_123" {
		t.Error("Payment hash not set correctly")
	}

	// Step 9: Admin fulfills payment
	fulfillReq := httptest.NewRequest(http.MethodPost, "/admin/payments/"+payment.ID+"/fulfill", nil)
	fulfillReq.Header.Set("X-Admin-Token", "test-admin-token")
	fulfillW := httptest.NewRecorder()
	r.ServeHTTP(fulfillW, fulfillReq)

	if fulfillW.Code != http.StatusOK {
		t.Errorf("Fulfill payment failed: %d - %s", fulfillW.Code, fulfillW.Body.String())
	}

	// Step 10: Verify fulfillment result
	payment, err = s.GetPayment(ctx, payment.ID)
	if err != nil {
		t.Fatalf("Failed to retrieve payment: %v", err)
	}
	if payment.Status != "fulfilled" {
		t.Errorf("Expected payment status fulfilled, got %s", payment.Status)
	}
	if payment.FulfillmentResult == nil || len(payment.FulfillmentResult) == 0 {
		t.Error("Fulfillment result not set")
	}
	if payment.FulfilledAt == nil {
		t.Error("FulfilledAt timestamp not set")
	}

	// Step 11: Check final payment status via API
	finalStatusReq := httptest.NewRequest(http.MethodGet, "/api/payment/"+payment.ID+"/status", nil)
	finalStatusW := httptest.NewRecorder()
	r.ServeHTTP(finalStatusW, finalStatusReq)

	if finalStatusW.Code != http.StatusOK {
		t.Errorf("Final payment status request failed: %d", finalStatusW.Code)
	}

	var finalStatusResp struct {
		Status            string                 `json:"status"`
		FulfillmentResult map[string]interface{} `json:"fulfillment_result"`
	}
	json.NewDecoder(finalStatusW.Body).Decode(&finalStatusResp)
	if finalStatusResp.Status != "fulfilled" {
		t.Errorf("Expected fulfilled status, got %s", finalStatusResp.Status)
	}
	if finalStatusResp.FulfillmentResult == nil {
		t.Error("Fulfillment result missing from status response")
	}

	t.Logf("Integration test passed: Full payment lifecycle from creation to fulfillment")
}

// TestPaymentFlow_WithFormSubmission tests the payment flow with form submission
func TestPaymentFlow_WithFormSubmission(t *testing.T) {
	os.Setenv("STORE_ADMIN_TOKEN", "test-admin-token")
	defer os.Unsetenv("STORE_ADMIN_TOKEN")

	_, db, s, r := setupIntegrationTest(t)
	ctx := context.Background()

	// Create category and item
	category := createCategory(db, "Physical", "Physical items")
	item := createItem(db, category.ID, "T-Shirt", "Custom t-shirt", "25.00", "BTC", "mock", models.JSONMap{})

	// Create payment
	payment := createPayment(db, item.ID, "25.00", "BTC")

	// Confirm payment
	err := s.ConfirmPayment(ctx, payment.ID, "payment_hash_456")
	if err != nil {
		t.Fatalf("Failed to confirm payment: %v", err)
	}

	// Submit form data (e.g., shipping address)
	formData := map[string]interface{}{
		"name":    "John Doe",
		"address": "123 Main St",
		"city":    "New York",
		"zip":     "10001",
	}
	formJSON, _ := json.Marshal(formData)
	formReq := httptest.NewRequest(http.MethodPost, "/api/payment/"+payment.ID+"/submit-form", bytes.NewReader(formJSON))
	formW := httptest.NewRecorder()
	r.ServeHTTP(formW, formReq)

	if formW.Code != http.StatusCreated {
		t.Errorf("Form submission failed: %d - %s", formW.Code, formW.Body.String())
	}

	// Verify form was stored
	var submission models.FormSubmission
	err = db.Where("payment_id = ?", payment.ID).First(&submission).Error
	if err != nil {
		t.Errorf("Form submission not found in database: %v", err)
	}

	if submission.FormData["name"] != "John Doe" {
		t.Error("Form data not stored correctly")
	}

	// Fulfill payment
	err = s.FulfillPayment(ctx, payment.ID)
	if err != nil {
		t.Fatalf("Failed to fulfill payment: %v", err)
	}

	// Verify final state
	payment, err = s.GetPayment(ctx, payment.ID)
	if err != nil {
		t.Fatalf("Failed to get payment: %v", err)
	}
	if payment.Status != "fulfilled" {
		t.Errorf("Expected fulfilled status, got %s", payment.Status)
	}

	t.Logf("Integration test passed: Payment flow with form submission")
}

// TestPaymentFlow_DigitalDownload tests digital media download flow
func TestPaymentFlow_DigitalDownload(t *testing.T) {
	_, db, s, r := setupIntegrationTest(t)
	ctx := context.Background()

	// Create digital item with download limit
	category := createCategory(db, "Digital", "Digital items")
	item := createItem(db, category.ID, "E-Book", "Digital book", "10.00", "BTC", "mock", models.JSONMap{
		"max_downloads": float64(3),
		"download_url":  "https://example.com/download/ebook.pdf",
	})

	// Create and confirm payment
	payment := createPayment(db, item.ID, "10.00", "BTC")
	s.ConfirmPayment(ctx, payment.ID, "hash_789")

	// Fulfill payment
	err := s.FulfillPayment(ctx, payment.ID)
	if err != nil {
		t.Fatalf("Failed to fulfill payment: %v", err)
	}

	// Track downloads
	for i := 1; i <= 3; i++ {
		trackReq := httptest.NewRequest(http.MethodPost, "/api/payment/"+payment.ID+"/track-download", nil)
		trackW := httptest.NewRecorder()
		r.ServeHTTP(trackW, trackReq)

		if trackW.Code != http.StatusOK {
			t.Errorf("Download tracking %d failed: %d", i, trackW.Code)
		}
	}

	// Verify download count
	var downloadCount int64
	db.Model(&models.DownloadLog{}).Where("payment_id = ?", payment.ID).Count(&downloadCount)
	if downloadCount != 3 {
		t.Errorf("Expected 3 downloads, got %d", downloadCount)
	}

	// Try to exceed limit
	trackReq := httptest.NewRequest(http.MethodPost, "/api/payment/"+payment.ID+"/track-download", nil)
	trackW := httptest.NewRecorder()
	r.ServeHTTP(trackW, trackReq)

	if trackW.Code != http.StatusTooManyRequests {
		t.Errorf("Expected 429 Too Many Requests when exceeding limit, got %d", trackW.Code)
	}

	t.Logf("Integration test passed: Digital download flow with limits")
}

// TestPaymentFlow_ExpiredDownload tests download expiration
func TestPaymentFlow_ExpiredDownload(t *testing.T) {
	_, db, s, r := setupIntegrationTest(t)
	ctx := context.Background()

	// Create digital item
	category := createCategory(db, "Digital", "Digital items")
	item := createItem(db, category.ID, "E-Book", "Digital book", "10.00", "BTC", "mock", models.JSONMap{})

	// Create and confirm payment
	payment := createPayment(db, item.ID, "10.00", "BTC")
	s.ConfirmPayment(ctx, payment.ID, "hash_exp")
	s.FulfillPayment(ctx, payment.ID)

	// Manually set expiration to past
	payment, _ = s.GetPayment(ctx, payment.ID)
	payment.FulfillmentResult["expires_at"] = time.Now().Add(-1 * time.Hour).Format(time.RFC3339)
	s.UpdateFulfillmentResult(ctx, payment.ID, payment.FulfillmentResult)

	// Try to download expired content
	trackReq := httptest.NewRequest(http.MethodPost, "/api/payment/"+payment.ID+"/track-download", nil)
	trackW := httptest.NewRecorder()
	r.ServeHTTP(trackW, trackReq)

	if trackW.Code != http.StatusGone {
		t.Errorf("Expected 410 Gone for expired download, got %d", trackW.Code)
	}

	t.Logf("Integration test passed: Expired download handling")
}

// TestPaymentFlow_ListPayments tests admin payment listing
func TestPaymentFlow_ListPayments(t *testing.T) {
	os.Setenv("STORE_ADMIN_TOKEN", "test-admin-token")
	defer os.Unsetenv("STORE_ADMIN_TOKEN")

	_, db, s, r := setupIntegrationTest(t)
	ctx := context.Background()

	// Create multiple payments
	category := createCategory(db, "Test", "Test")
	item := createItem(db, category.ID, "Item", "Test item", "10.00", "BTC", "mock", models.JSONMap{})

	for i := 0; i < 5; i++ {
		payment := createPayment(db, item.ID, "10.00", "BTC")
		if i%2 == 0 {
			s.ConfirmPayment(ctx, payment.ID, "hash_"+string(rune(i)))
		}
	}

	// List payments via API
	listReq := httptest.NewRequest(http.MethodGet, "/admin/payments", nil)
	listReq.Header.Set("X-Admin-Token", "test-admin-token")
	listW := httptest.NewRecorder()
	r.ServeHTTP(listW, listReq)

	if listW.Code != http.StatusOK {
		t.Errorf("List payments failed: %d", listW.Code)
	}

	var listResp struct {
		Payments []models.Payment `json:"payments"`
	}
	json.NewDecoder(listW.Body).Decode(&listResp)

	if len(listResp.Payments) != 5 {
		t.Errorf("Expected 5 payments, got %d", len(listResp.Payments))
	}

	// Verify status mix
	confirmedCount := 0
	for _, p := range listResp.Payments {
		if p.Status == "confirmed" {
			confirmedCount++
		}
	}
	if confirmedCount != 3 {
		t.Errorf("Expected 3 confirmed payments, got %d", confirmedCount)
	}

	t.Logf("Integration test passed: Admin payment listing")
}
