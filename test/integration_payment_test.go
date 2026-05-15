package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gorilla/mux"
	bolt "go.etcd.io/bbolt"

	"github.com/opd-ai/store/internal/api"
	"github.com/opd-ai/store/pkg/config"
	"github.com/opd-ai/store/pkg/db"
	"github.com/opd-ai/store/pkg/handler"
	"github.com/opd-ai/store/pkg/models"
	"github.com/opd-ai/store/pkg/paywall"
	"github.com/opd-ai/store/pkg/store"
)

// mockIntegrationPaywallClient implements paywall.Service for integration tests
type mockIntegrationPaywallClient struct{}

func (m *mockIntegrationPaywallClient) CreateInvoice(ctx context.Context, amount, currency, callbackURL string) (*paywall.Invoice, error) {
	return &paywall.Invoice{
		InvoiceID:      "test-invoice-" + amount,
		Status:         "pending",
		PaymentAddress: "bc1qtest",
		QRCode:         "data:image/png;base64,test",
		ExpiresAt:      time.Now().Add(30 * time.Minute),
	}, nil
}

func (m *mockIntegrationPaywallClient) GetInvoiceStatus(ctx context.Context, invoiceID string) (*paywall.InvoiceStatus, error) {
	return &paywall.InvoiceStatus{
		InvoiceID: invoiceID,
		Status:    "confirmed",
		Confirmed: true,
	}, nil
}

func (m *mockIntegrationPaywallClient) VerifyWebhook(signature string, payload []byte, secret string) (bool, error) {
	return true, nil
}

func (m *mockIntegrationPaywallClient) CreateEmbeddedPayment(ctx context.Context, amount float64, timeout time.Duration, useEscrow bool) (*paywall.EmbeddedPayment, error) {
	return &paywall.EmbeddedPayment{
		ID:            "test-payment-id",
		Status:        "pending",
		Address:       "bc1qtest",
		Amount:        amount,
		Currency:      "BTC",
		EscrowEnabled: useEscrow,
		ExpiresAt:     time.Now().Add(30 * time.Minute),
	}, nil
}

func (m *mockIntegrationPaywallClient) ConfirmEmbeddedPayment(ctx context.Context, paymentID, paymentHash string) error {
	return nil
}

func (m *mockIntegrationPaywallClient) GetEmbeddedPayment(ctx context.Context, paymentID string) (*paywall.EmbeddedPayment, error) {
	return &paywall.EmbeddedPayment{
		ID:        paymentID,
		Status:    "confirmed",
		Address:   "bc1qtest",
		Amount:    100000,
		Currency:  "BTC",
		ExpiresAt: time.Now().Add(30 * time.Minute),
	}, nil
}

func (m *mockIntegrationPaywallClient) ReleaseEscrow(ctx context.Context, paymentID string, signatures []paywall.SignatureData) error {
	return nil
}

func (m *mockIntegrationPaywallClient) RefundEscrow(ctx context.Context, paymentID string, signatures []paywall.SignatureData) error {
	return nil
}

func (m *mockIntegrationPaywallClient) DisputeEscrow(ctx context.Context, paymentID string, reason string) error {
	return nil
}

func (m *mockIntegrationPaywallClient) ResolveDispute(ctx context.Context, paymentID string, resolution string, arbiterSig paywall.SignatureData) error {
	return nil
}

// setupIntegrationTest creates test infrastructure for integration tests
func setupIntegrationTest(t *testing.T) (*api.Handler, *bolt.DB, *store.Store, *mux.Router) {
	// Create temporary database
	tmpFile := "/tmp/test_integration_" + t.Name() + ".db"
	t.Cleanup(func() {
		os.Remove(tmpFile)
	})

	boltDB, err := bolt.Open(tmpFile, 0o600, nil)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	t.Cleanup(func() {
		boltDB.Close()
	})

	// Initialize buckets
	if err := db.InitBuckets(boltDB); err != nil {
		t.Fatalf("Failed to initialize buckets: %v", err)
	}

	// Create handler registry with mock handler
	reg := handler.NewRegistry()
	mockHandler := &testIntegrationHandler{
		handlerType: "mock",
	}
	if err := reg.Register(mockHandler); err != nil {
		t.Fatalf("Failed to register handler: %v", err)
	}

	// Wrap BoltDB in Database interface
	database := db.NewBoltDatabase(boltDB)

	// Create store
	s := store.NewStore(database, reg)

	// Create mock paywall client (for integration tests, we still mock external services)
	paywallClient := &mockIntegrationPaywallClient{}

	testCfg := &config.Config{}
	h := api.NewHandler(s, paywallClient, "test-admin-token", testCfg)

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

	return h, boltDB, s, r
}

// Helper functions for creating test data
func createCategory(s *store.Store, name, description string) *models.Category {
	cat, _ := s.CreateCategory(context.Background(), name, description)
	return cat
}

func createItem(s *store.Store, categoryID, name, description, price, currency, backendType string, config models.JSONMap) *models.Item {
	item := models.NewItem(categoryID, name, description, price, currency, backendType)
	item.BackendConfig = config
	s.CreateItem(context.Background(), item)
	return item
}

func createPayment(s *store.Store, itemID, amount, currency string) *models.Payment {
	payment, _ := s.CreatePayment(context.Background(), itemID, amount, currency)
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

	_, _, s, r := setupIntegrationTest(t)
	ctx := context.Background()

	// Step 1: Create a category
	category := createCategory(s, "Electronics", "Electronic items")

	// Step 2: Create an item
	item := createItem(s, category.ID, "Laptop", "High-end laptop", "1000.00", "BTC", "mock", models.JSONMap{
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
	payment := createPayment(s, item.ID, "1000.00", "BTC")

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

	_, _, s, r := setupIntegrationTest(t)
	ctx := context.Background()

	// Create category and item
	category := createCategory(s, "Physical", "Physical items")
	item := createItem(s, category.ID, "T-Shirt", "Custom t-shirt", "25.00", "BTC", "mock", models.JSONMap{})

	// Create payment
	payment := createPayment(s, item.ID, "25.00", "BTC")

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
	submission, err := s.GetFormSubmission(ctx, payment.ID)
	if err != nil {
		t.Errorf("Form submission not found: %v", err)
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
	_, _, s, r := setupIntegrationTest(t)
	ctx := context.Background()

	// Create digital item with download limit
	category := createCategory(s, "Digital", "Digital items")
	item := createItem(s, category.ID, "E-Book", "Digital book", "10.00", "BTC", "mock", models.JSONMap{
		"max_downloads": float64(3),
		"download_url":  "https://example.com/download/ebook.pdf",
	})

	// Create and confirm payment
	payment := createPayment(s, item.ID, "10.00", "BTC")
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
	downloadCount, err := s.GetDownloadCount(ctx, payment.ID)
	if err != nil {
		t.Errorf("Failed to get download count: %v", err)
	}
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
	_, _, s, r := setupIntegrationTest(t)
	ctx := context.Background()

	// Create digital item
	category := createCategory(s, "Digital", "Digital items")
	item := createItem(s, category.ID, "E-Book", "Digital book", "10.00", "BTC", "mock", models.JSONMap{})

	// Create and confirm payment
	payment := createPayment(s, item.ID, "10.00", "BTC")
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

	_, _, s, r := setupIntegrationTest(t)
	ctx := context.Background()

	// Create multiple payments
	category := createCategory(s, "Test", "Test")
	item := createItem(s, category.ID, "Item", "Test item", "10.00", "BTC", "mock", models.JSONMap{})

	for i := 0; i < 5; i++ {
		payment := createPayment(s, item.ID, "10.00", "BTC")
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

// TestPaymentFlow_CheckoutEndpoint tests the actual /api/checkout endpoint
func TestPaymentFlow_CheckoutEndpoint(t *testing.T) {
	os.Setenv("STORE_ADMIN_TOKEN", "test-admin-token")
	os.Setenv("STORE_PUBLIC_URL", "http://localhost:8080")
	defer os.Unsetenv("STORE_ADMIN_TOKEN")
	defer os.Unsetenv("STORE_PUBLIC_URL")

	// Create mock paywall server
	mockPaywall := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/create-payment" {
			response := map[string]interface{}{
				"invoice_id":      "test-invoice-123",
				"status":          "pending",
				"payment_address": "bc1qtest123456",
				"qr_code":         "data:image/png;base64,test",
				"expires_at":      time.Now().Add(30 * time.Minute).Format(time.RFC3339),
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		} else if r.URL.Path == "/invoice-status" {
			// Return pending status for invoice status requests
			response := map[string]interface{}{
				"invoice_id": "test-invoice-123",
				"status":     "pending",
				"confirmed":  false,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}
	}))
	defer mockPaywall.Close()

	// Create test infrastructure with mock paywall URL
	tmpFile := "/tmp/test_int_" + t.Name() + ".db"
	t.Cleanup(func() { os.Remove(tmpFile) })
	boltDB, err := bolt.Open(tmpFile, 0o600, nil)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	if err := db.InitBuckets(boltDB); err != nil {
		t.Fatalf("Failed to initialize buckets: %v", err)
	}

	t.Cleanup(func() { boltDB.Close() })

	reg := handler.NewRegistry()
	mockHandler := &testIntegrationHandler{handlerType: "mock"}
	if err := reg.Register(mockHandler); err != nil {
		t.Fatalf("Failed to register handler: %v", err)
	}

	database := db.NewBoltDatabase(boltDB)
	s := store.NewStore(database, reg)
	paywallClient := &mockIntegrationPaywallClient{}
	testCfg := &config.Config{}
	h := api.NewHandler(s, paywallClient, "test-admin-token", testCfg)

	r := mux.NewRouter()
	r.HandleFunc("/api/catalog", h.GetCatalog).Methods("GET")
	r.HandleFunc("/api/checkout", h.CreateCheckout).Methods("POST")
	r.HandleFunc("/api/payment/{id}/status", h.GetPaymentStatus).Methods("GET")
	r.HandleFunc("/admin/payments/{id}/confirm", h.ConfirmPayment).Methods("POST")

	ctx := context.Background()

	// Create category and item
	category := createCategory(s, "Test Products", "Test category")
	item := createItem(s, category.ID, "Test Product", "A test product", "50.00", "BTC", "mock", models.JSONMap{})

	// POST to /api/checkout
	checkoutReq := map[string]string{
		"item_id": item.ID,
		"email":   "buyer@example.com",
	}
	checkoutJSON, _ := json.Marshal(checkoutReq)
	checkoutHTTPReq := httptest.NewRequest(http.MethodPost, "/api/checkout", bytes.NewReader(checkoutJSON))
	checkoutHTTPReq.Header.Set("Content-Type", "application/json")
	checkoutW := httptest.NewRecorder()
	r.ServeHTTP(checkoutW, checkoutHTTPReq)

	if checkoutW.Code != http.StatusOK && checkoutW.Code != http.StatusCreated {
		t.Fatalf("Checkout failed: %d - %s", checkoutW.Code, checkoutW.Body.String())
	}

	var checkoutResp struct {
		PaymentID      string `json:"payment_id"`
		Amount         string `json:"amount"`
		Currency       string `json:"currency"`
		PaymentAddress string `json:"payment_address"`
		InvoiceID      string `json:"invoice_id"`
	}
	json.NewDecoder(checkoutW.Body).Decode(&checkoutResp)

	if checkoutResp.PaymentID == "" {
		t.Fatal("Expected payment_id in checkout response")
	}
	if checkoutResp.InvoiceID == "" {
		t.Fatal("Expected invoice_id in checkout response")
	}

	// GET /api/payment/{id}/status
	statusReq := httptest.NewRequest(http.MethodGet, "/api/payment/"+checkoutResp.PaymentID+"/status", nil)
	statusW := httptest.NewRecorder()
	r.ServeHTTP(statusW, statusReq)

	if statusW.Code != http.StatusOK {
		t.Errorf("Payment status request failed: %d", statusW.Code)
	}

	var statusResp struct {
		Status   string `json:"status"`
		Amount   string `json:"amount"`
		Currency string `json:"currency"`
	}
	json.NewDecoder(statusW.Body).Decode(&statusResp)

	if statusResp.Status != "pending" {
		t.Errorf("Expected status 'pending', got %s", statusResp.Status)
	}
	if statusResp.Amount != "50.00" {
		t.Errorf("Expected amount '50.00', got %s", statusResp.Amount)
	}

	// Verify payment in database
	payment, err := s.GetPayment(ctx, checkoutResp.PaymentID)
	if err != nil {
		t.Fatalf("Failed to get payment: %v", err)
	}
	if payment.PayerInfo["email"] != "buyer@example.com" {
		t.Error("Expected email to be stored in payer_info")
	}

	t.Logf("Integration test passed: Checkout endpoint flow")
}

// setupTestWithHandlers creates test infrastructure with real handlers
func setupTestWithHandlers(t *testing.T) (*api.Handler, *bolt.DB, *store.Store, *mux.Router, *httptest.Server) {
	// Create mock paywall server
	mockPaywall := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/create-payment" {
			response := map[string]interface{}{
				"invoice_id":      "test-invoice-" + time.Now().Format("20060102150405"),
				"status":          "pending",
				"payment_address": "bc1qtest" + time.Now().Format("20060102150405"),
				"qr_code":         "data:image/png;base64,test",
				"expires_at":      time.Now().Add(30 * time.Minute).Format(time.RFC3339),
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		} else if r.URL.Path == "/invoice-status" {
			// Return pending status for invoice status requests
			response := map[string]interface{}{
				"invoice_id": "test-invoice",
				"status":     "pending",
				"confirmed":  false,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}
	}))

	// Create in-memory database
	tmpFile := "/tmp/test_int_" + t.Name() + ".db"
	t.Cleanup(func() { os.Remove(tmpFile) })
	boltDB, err := bolt.Open(tmpFile, 0o600, nil)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	// Run migrations
	if err := db.InitBuckets(boltDB); err != nil {
		t.Fatalf("Failed to initialize buckets: %v", err)
	}

	t.Cleanup(func() { boltDB.Close() })

	// Create handler registry with real handlers
	reg := handler.NewRegistry()
	handlers := []handler.FulfillmentHandler{
		&testDigitalMediaHandler{},
		&testShippingFormHandler{},
		&testPrintOnDemandHandler{},
		&testCustomHandler{},
	}
	for _, h := range handlers {
		if err := reg.Register(h); err != nil {
			t.Fatalf("Failed to register handler: %v", err)
		}
	}

	// Wrap BoltDB in Database interface
	database := db.NewBoltDatabase(boltDB)

	// Create store
	s := store.NewStore(database, reg)

	// Create mock paywall client with mock server URL
	paywallClient := &mockIntegrationPaywallClient{}

	testCfg := &config.Config{}
	h := api.NewHandler(s, paywallClient, "test-admin-token", testCfg)

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

	// Admin routes
	r.HandleFunc("/admin/payments", h.ListPayments).Methods("GET")
	r.HandleFunc("/admin/payments/{id}/confirm", h.ConfirmPayment).Methods("POST")
	r.HandleFunc("/admin/payments/{id}/fulfill", h.FulfillPayment).Methods("POST")
	r.HandleFunc("/admin/categories", h.CreateCategory).Methods("POST")
	r.HandleFunc("/admin/categories", h.ListCategories).Methods("GET")
	r.HandleFunc("/admin/items", h.CreateItem).Methods("POST")
	r.HandleFunc("/admin/items", h.ListItems).Methods("GET")

	return h, boltDB, s, r, mockPaywall
}

// Test handler implementations that mimic real behavior

type testDigitalMediaHandler struct{}

func (h *testDigitalMediaHandler) Metadata() handler.HandlerMetadata {
	return handler.HandlerMetadata{
		Type:        "digital_media",
		DisplayName: "Digital Media",
		Description: "Test digital media handler",
	}
}

func (h *testDigitalMediaHandler) Validate(config models.JSONMap) error {
	storage, ok := config["storage"].(string)
	if !ok || (storage != "local" && storage != "s3") {
		return fmt.Errorf("storage must be 'local' or 's3'")
	}
	if _, ok := config["file_path"].(string); !ok {
		return fmt.Errorf("file_path is required")
	}
	return nil
}

func (h *testDigitalMediaHandler) RequiresForm() bool {
	return false
}

func (h *testDigitalMediaHandler) GetFormSchema() map[string]interface{} {
	return nil
}

func (h *testDigitalMediaHandler) Handle(ctx context.Context, payment *models.Payment, item *models.Item) (map[string]interface{}, error) {
	filePath := item.BackendConfig["file_path"].(string)
	return map[string]interface{}{
		"download_url": "http://example.com/download/" + filePath,
		"expires_at":   time.Now().Add(24 * time.Hour).Format(time.RFC3339),
	}, nil
}

type testShippingFormHandler struct{}

func (h *testShippingFormHandler) Metadata() handler.HandlerMetadata {
	return handler.HandlerMetadata{
		Type:        "shipping_form",
		DisplayName: "Shipping Form",
		Description: "Test shipping form handler",
	}
}

func (h *testShippingFormHandler) Validate(config models.JSONMap) error {
	return nil
}

func (h *testShippingFormHandler) RequiresForm() bool {
	return true
}

func (h *testShippingFormHandler) GetFormSchema() map[string]interface{} {
	return map[string]interface{}{
		"fields": []string{"name", "address", "city", "state", "zip", "country"},
	}
}

func (h *testShippingFormHandler) Handle(ctx context.Context, payment *models.Payment, item *models.Item) (map[string]interface{}, error) {
	return map[string]interface{}{
		"status":  "awaiting_shipment",
		"message": "Order will be shipped within 2-3 business days",
	}, nil
}

type testPrintOnDemandHandler struct{}

func (h *testPrintOnDemandHandler) Metadata() handler.HandlerMetadata {
	return handler.HandlerMetadata{
		Type:        "pod",
		DisplayName: "Print on Demand",
		Description: "Test POD handler",
	}
}

func (h *testPrintOnDemandHandler) Validate(config models.JSONMap) error {
	provider, ok := config["provider"].(string)
	if !ok || provider != "printful" {
		return fmt.Errorf("invalid provider")
	}
	if _, ok := config["product_id"].(string); !ok {
		return fmt.Errorf("product_id is required")
	}
	return nil
}

func (h *testPrintOnDemandHandler) RequiresForm() bool {
	return true
}

func (h *testPrintOnDemandHandler) GetFormSchema() map[string]interface{} {
	return map[string]interface{}{
		"fields": []string{"name", "address", "city", "state", "zip", "country", "size"},
	}
}

func (h *testPrintOnDemandHandler) Handle(ctx context.Context, payment *models.Payment, item *models.Item) (map[string]interface{}, error) {
	provider := item.BackendConfig["provider"].(string)
	return map[string]interface{}{
		"order_id":     "TEST-ORDER-123",
		"provider":     provider,
		"status":       "processing",
		"tracking_url": "http://example.com/track/123",
	}, nil
}

type testCustomHandler struct{}

func (h *testCustomHandler) Metadata() handler.HandlerMetadata {
	return handler.HandlerMetadata{
		Type:        "custom",
		DisplayName: "Custom Webhook",
		Description: "Test custom handler",
	}
}

func (h *testCustomHandler) Validate(config models.JSONMap) error {
	if _, ok := config["webhook_url"].(string); !ok {
		return fmt.Errorf("webhook_url is required")
	}
	return nil
}

func (h *testCustomHandler) RequiresForm() bool {
	return false
}

func (h *testCustomHandler) GetFormSchema() map[string]interface{} {
	return nil
}

func (h *testCustomHandler) Handle(ctx context.Context, payment *models.Payment, item *models.Item) (map[string]interface{}, error) {
	webhookURL := item.BackendConfig["webhook_url"].(string)
	return map[string]interface{}{
		"webhook_called": true,
		"webhook_url":    webhookURL,
		"response":       "success",
	}, nil
}

// TestPaymentFlow_DigitalMediaHandlerEndToEnd tests digital media handler with checkout
func TestPaymentFlow_DigitalMediaHandlerEndToEnd(t *testing.T) {
	os.Setenv("STORE_ADMIN_TOKEN", "test-admin-token")
	os.Setenv("STORE_PUBLIC_URL", "http://localhost:8080")
	defer os.Unsetenv("STORE_ADMIN_TOKEN")
	defer os.Unsetenv("STORE_PUBLIC_URL")

	_, _, s, r, mockPaywall := setupTestWithHandlers(t)
	defer mockPaywall.Close()
	ctx := context.Background()

	// Create item with digital_media backend
	category := createCategory(s, "Digital", "Digital products")
	item := createItem(s, category.ID, "E-Book", "Test e-book", "10.00", "BTC", "digital_media", models.JSONMap{
		"storage":          "local",
		"file_path":        "/downloads/ebook.pdf",
		"expiration_hours": 24,
	})

	// Checkout
	checkoutReq := map[string]string{"item_id": item.ID, "email": "user@example.com"}
	checkoutJSON, _ := json.Marshal(checkoutReq)
	checkoutHTTPReq := httptest.NewRequest(http.MethodPost, "/api/checkout", bytes.NewReader(checkoutJSON))
	checkoutW := httptest.NewRecorder()
	r.ServeHTTP(checkoutW, checkoutHTTPReq)

	if checkoutW.Code != http.StatusOK && checkoutW.Code != http.StatusCreated {
		t.Fatalf("Checkout failed: %d - %s", checkoutW.Code, checkoutW.Body.String())
	}

	var checkoutResp struct {
		PaymentID string `json:"payment_id"`
	}
	json.NewDecoder(checkoutW.Body).Decode(&checkoutResp)

	// Confirm payment
	err := s.ConfirmPayment(ctx, checkoutResp.PaymentID, "payment_hash_digital")
	if err != nil {
		t.Fatalf("Failed to confirm payment: %v", err)
	}

	// Fulfill payment
	err = s.FulfillPayment(ctx, checkoutResp.PaymentID)
	if err != nil {
		t.Fatalf("Failed to fulfill payment: %v", err)
	}

	// Verify fulfillment result
	payment, err := s.GetPayment(ctx, checkoutResp.PaymentID)
	if err != nil {
		t.Fatalf("Failed to get payment: %v", err)
	}

	if payment.Status != "fulfilled" {
		t.Errorf("Expected status 'fulfilled', got %s", payment.Status)
	}

	downloadURL, ok := payment.FulfillmentResult["download_url"].(string)
	if !ok || downloadURL == "" {
		t.Error("Expected download_url in fulfillment result")
	}

	t.Logf("Integration test passed: Digital media handler end-to-end")
}

// TestPaymentFlow_ShippingFormHandlerEndToEnd tests shipping form handler with form submission
func TestPaymentFlow_ShippingFormHandlerEndToEnd(t *testing.T) {
	os.Setenv("STORE_ADMIN_TOKEN", "test-admin-token")
	os.Setenv("STORE_PUBLIC_URL", "http://localhost:8080")
	defer os.Unsetenv("STORE_ADMIN_TOKEN")
	defer os.Unsetenv("STORE_PUBLIC_URL")

	_, _, s, r, mockPaywall := setupTestWithHandlers(t)
	defer mockPaywall.Close()
	ctx := context.Background()

	// Create item with shipping_form backend
	category := createCategory(s, "Physical", "Physical products")
	item := createItem(s, category.ID, "T-Shirt", "Custom t-shirt", "25.00", "BTC", "shipping_form", models.JSONMap{})

	// Checkout
	checkoutReq := map[string]string{"item_id": item.ID, "email": "buyer@example.com"}
	checkoutJSON, _ := json.Marshal(checkoutReq)
	checkoutHTTPReq := httptest.NewRequest(http.MethodPost, "/api/checkout", bytes.NewReader(checkoutJSON))
	checkoutW := httptest.NewRecorder()
	r.ServeHTTP(checkoutW, checkoutHTTPReq)

	if checkoutW.Code != http.StatusOK && checkoutW.Code != http.StatusCreated {
		t.Fatalf("Checkout failed: %d", checkoutW.Code)
	}

	var checkoutResp struct {
		PaymentID string `json:"payment_id"`
	}
	json.NewDecoder(checkoutW.Body).Decode(&checkoutResp)

	// Confirm payment
	err := s.ConfirmPayment(ctx, checkoutResp.PaymentID, "payment_hash_shipping")
	if err != nil {
		t.Fatalf("Failed to confirm payment: %v", err)
	}

	// Submit shipping form
	formData := map[string]interface{}{
		"name":    "John Doe",
		"address": "123 Main St",
		"city":    "New York",
		"state":   "NY",
		"zip":     "10001",
		"country": "USA",
	}
	formJSON, _ := json.Marshal(formData)
	formReq := httptest.NewRequest(http.MethodPost, "/api/payment/"+checkoutResp.PaymentID+"/submit-form", bytes.NewReader(formJSON))
	formW := httptest.NewRecorder()
	r.ServeHTTP(formW, formReq)

	if formW.Code != http.StatusOK && formW.Code != http.StatusCreated {
		t.Errorf("Form submission failed: %d - %s", formW.Code, formW.Body.String())
	}

	// Fulfill payment
	err = s.FulfillPayment(ctx, checkoutResp.PaymentID)
	if err != nil {
		t.Fatalf("Failed to fulfill payment: %v", err)
	}

	// Verify fulfillment
	payment, err := s.GetPayment(ctx, checkoutResp.PaymentID)
	if err != nil {
		t.Fatalf("Failed to get payment: %v", err)
	}

	if payment.Status != "fulfilled" {
		t.Errorf("Expected status 'fulfilled', got %s", payment.Status)
	}

	if payment.FulfillmentResult["status"] != "awaiting_shipment" {
		t.Error("Expected 'awaiting_shipment' status in fulfillment result")
	}

	t.Logf("Integration test passed: Shipping form handler end-to-end with form submission")
}

// TestPaymentFlow_PODHandlerEndToEnd tests print-on-demand handler
func TestPaymentFlow_PODHandlerEndToEnd(t *testing.T) {
	os.Setenv("STORE_ADMIN_TOKEN", "test-admin-token")
	os.Setenv("STORE_PUBLIC_URL", "http://localhost:8080")
	defer os.Unsetenv("STORE_ADMIN_TOKEN")
	defer os.Unsetenv("STORE_PUBLIC_URL")

	_, _, s, r, mockPaywall := setupTestWithHandlers(t)
	defer mockPaywall.Close()
	ctx := context.Background()

	// Create item with pod backend
	category := createCategory(s, "Apparel", "Print on demand apparel")
	item := createItem(s, category.ID, "Custom Mug", "Custom printed mug", "15.00", "BTC", "pod", models.JSONMap{
		"provider":   "printful",
		"product_id": "PROD-123",
		"variant_id": "VAR-456",
	})

	// Checkout
	checkoutReq := map[string]string{"item_id": item.ID, "email": "customer@example.com"}
	checkoutJSON, _ := json.Marshal(checkoutReq)
	checkoutHTTPReq := httptest.NewRequest(http.MethodPost, "/api/checkout", bytes.NewReader(checkoutJSON))
	checkoutW := httptest.NewRecorder()
	r.ServeHTTP(checkoutW, checkoutHTTPReq)

	if checkoutW.Code != http.StatusOK && checkoutW.Code != http.StatusCreated {
		t.Fatalf("Checkout failed: %d", checkoutW.Code)
	}

	var checkoutResp struct {
		PaymentID string `json:"payment_id"`
	}
	json.NewDecoder(checkoutW.Body).Decode(&checkoutResp)

	// Confirm payment
	err := s.ConfirmPayment(ctx, checkoutResp.PaymentID, "payment_hash_pod")
	if err != nil {
		t.Fatalf("Failed to confirm payment: %v", err)
	}

	// Submit shipping form (POD requires shipping info)
	formData := map[string]interface{}{
		"name":    "Jane Smith",
		"address": "456 Oak Ave",
		"city":    "Los Angeles",
		"state":   "CA",
		"zip":     "90001",
		"country": "USA",
		"size":    "M",
	}
	formJSON, _ := json.Marshal(formData)
	formReq := httptest.NewRequest(http.MethodPost, "/api/payment/"+checkoutResp.PaymentID+"/submit-form", bytes.NewReader(formJSON))
	formW := httptest.NewRecorder()
	r.ServeHTTP(formW, formReq)

	if formW.Code != http.StatusOK && formW.Code != http.StatusCreated {
		t.Errorf("Form submission failed: %d", formW.Code)
	}

	// Fulfill payment
	err = s.FulfillPayment(ctx, checkoutResp.PaymentID)
	if err != nil {
		t.Fatalf("Failed to fulfill payment: %v", err)
	}

	// Verify fulfillment
	payment, err := s.GetPayment(ctx, checkoutResp.PaymentID)
	if err != nil {
		t.Fatalf("Failed to get payment: %v", err)
	}

	if payment.Status != "fulfilled" {
		t.Errorf("Expected status 'fulfilled', got %s", payment.Status)
	}

	orderID, ok := payment.FulfillmentResult["order_id"].(string)
	if !ok || orderID == "" {
		t.Error("Expected order_id in fulfillment result")
	}

	provider, ok := payment.FulfillmentResult["provider"].(string)
	if !ok || provider != "printful" {
		t.Error("Expected provider 'printful' in fulfillment result")
	}

	t.Logf("Integration test passed: POD handler end-to-end")
}

// TestPaymentFlow_CustomHandlerEndToEnd tests custom webhook handler
func TestPaymentFlow_CustomHandlerEndToEnd(t *testing.T) {
	os.Setenv("STORE_ADMIN_TOKEN", "test-admin-token")
	os.Setenv("STORE_PUBLIC_URL", "http://localhost:8080")
	defer os.Unsetenv("STORE_ADMIN_TOKEN")
	defer os.Unsetenv("STORE_PUBLIC_URL")

	_, _, s, r, mockPaywall := setupTestWithHandlers(t)
	defer mockPaywall.Close()
	ctx := context.Background()

	// Create item with custom backend
	category := createCategory(s, "Services", "Custom services")
	item := createItem(s, category.ID, "Consultation", "1-hour consultation", "100.00", "BTC", "custom", models.JSONMap{
		"webhook_url": "https://example.com/webhook/fulfill",
		"method":      "POST",
	})

	// Checkout
	checkoutReq := map[string]string{"item_id": item.ID, "email": "client@example.com"}
	checkoutJSON, _ := json.Marshal(checkoutReq)
	checkoutHTTPReq := httptest.NewRequest(http.MethodPost, "/api/checkout", bytes.NewReader(checkoutJSON))
	checkoutW := httptest.NewRecorder()
	r.ServeHTTP(checkoutW, checkoutHTTPReq)

	if checkoutW.Code != http.StatusOK && checkoutW.Code != http.StatusCreated {
		t.Fatalf("Checkout failed: %d", checkoutW.Code)
	}

	var checkoutResp struct {
		PaymentID string `json:"payment_id"`
	}
	json.NewDecoder(checkoutW.Body).Decode(&checkoutResp)

	// Confirm payment
	err := s.ConfirmPayment(ctx, checkoutResp.PaymentID, "payment_hash_custom")
	if err != nil {
		t.Fatalf("Failed to confirm payment: %v", err)
	}

	// Fulfill payment
	err = s.FulfillPayment(ctx, checkoutResp.PaymentID)
	if err != nil {
		t.Fatalf("Failed to fulfill payment: %v", err)
	}

	// Verify fulfillment
	payment, err := s.GetPayment(ctx, checkoutResp.PaymentID)
	if err != nil {
		t.Fatalf("Failed to get payment: %v", err)
	}

	if payment.Status != "fulfilled" {
		t.Errorf("Expected status 'fulfilled', got %s", payment.Status)
	}

	webhookCalled, ok := payment.FulfillmentResult["webhook_called"].(bool)
	if !ok || !webhookCalled {
		t.Error("Expected webhook_called=true in fulfillment result")
	}

	webhookURL, ok := payment.FulfillmentResult["webhook_url"].(string)
	if !ok || webhookURL != "https://example.com/webhook/fulfill" {
		t.Error("Expected webhook_url in fulfillment result")
	}

	t.Logf("Integration test passed: Custom handler end-to-end")
}
