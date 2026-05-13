package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/opd-ai/store/pkg/models"
)

func TestCustomHandler_Validate(t *testing.T) {
	handler := NewCustomHandler()

	tests := []struct {
		name    string
		config  models.JSONMap
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid https webhook_url",
			config: models.JSONMap{
				"webhook_url": "https://example.com/webhook",
			},
			wantErr: false,
		},
		{
			name: "valid http webhook_url",
			config: models.JSONMap{
				"webhook_url": "http://localhost:8080/webhook",
			},
			wantErr: false,
		},
		{
			name: "valid config with optional fields",
			config: models.JSONMap{
				"webhook_url":     "https://example.com/webhook",
				"retry_count":     3,
				"timeout_seconds": 30,
				"webhook_headers": map[string]interface{}{
					"Authorization": "Bearer token123",
				},
			},
			wantErr: false,
		},
		{
			name:    "missing webhook_url",
			config:  models.JSONMap{},
			wantErr: true,
			errMsg:  "missing required field: webhook_url",
		},
		{
			name: "empty webhook_url",
			config: models.JSONMap{
				"webhook_url": "",
			},
			wantErr: true,
			errMsg:  "missing required field: webhook_url",
		},
		{
			name: "invalid webhook_url (not http/https)",
			config: models.JSONMap{
				"webhook_url": "ftp://example.com/webhook",
			},
			wantErr: true,
			errMsg:  "invalid webhook_url: must start with http://",
		},
		{
			name: "invalid webhook_url type",
			config: models.JSONMap{
				"webhook_url": 12345,
			},
			wantErr: true,
			errMsg:  "missing required field: webhook_url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.Validate(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" && err != nil {
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error message = %v, want to contain %v", err.Error(), tt.errMsg)
				}
			}
		})
	}
}

func TestCustomHandler_Handle(t *testing.T) {
	// Create a test HTTP server
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method
		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		// Verify content type
		if r.Header.Get("Content-Type") != "application/json" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Parse request body
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Return success response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		response := map[string]interface{}{
			"status":  "success",
			"message": "Order processed",
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer testServer.Close()

	handler := NewCustomHandler()
	ctx := context.Background()

	paymentHash := "abc123"
	tests := []struct {
		name    string
		payment *models.Payment
		item    *models.Item
		wantErr bool
		errMsg  string
		check   func(*testing.T, map[string]interface{})
	}{
		{
			name: "successful webhook invocation",
			payment: &models.Payment{
				ID:          "pay_123",
				Status:      "confirmed",
				Amount:      "0.001",
				Currency:    "BTC",
				PaymentHash: &paymentHash,
				PayerInfo: models.JSONMap{
					"email": "test@example.com",
				},
			},
			item: &models.Item{
				ID:   "item_123",
				Name: "Test Product",
				BackendConfig: models.JSONMap{
					"webhook_url": testServer.URL,
					"retry_count": 2,
				},
			},
			wantErr: false,
			check: func(t *testing.T, result map[string]interface{}) {
				if result["status"] != "success" {
					t.Errorf("Expected status='success', got %v", result["status"])
				}
				if result["message"] != "Order processed" {
					t.Errorf("Expected message='Order processed', got %v", result["message"])
				}
			},
		},
		{
			name: "payment not confirmed",
			payment: &models.Payment{
				ID:     "pay_123",
				Status: "pending",
			},
			item: &models.Item{
				ID:   "item_123",
				Name: "Test Product",
				BackendConfig: models.JSONMap{
					"webhook_url": testServer.URL,
				},
			},
			wantErr: true,
			errMsg:  "payment not confirmed",
		},
		{
			name: "missing backend configuration",
			payment: &models.Payment{
				ID:     "pay_123",
				Status: "confirmed",
			},
			item: &models.Item{
				ID:            "item_123",
				Name:          "Test Product",
				BackendConfig: nil,
			},
			wantErr: true,
			errMsg:  "missing backend configuration",
		},
		{
			name: "missing webhook_url in config",
			payment: &models.Payment{
				ID:     "pay_123",
				Status: "confirmed",
			},
			item: &models.Item{
				ID:   "item_123",
				Name: "Test Product",
				BackendConfig: models.JSONMap{
					"retry_count": 2,
				},
			},
			wantErr: true,
			errMsg:  "missing or invalid webhook_url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := handler.Handle(ctx, tt.payment, tt.item)
			if (err != nil) != tt.wantErr {
				t.Errorf("Handle() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" && err != nil {
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("Handle() error message = %v, want to contain %v", err.Error(), tt.errMsg)
				}
			}
			if !tt.wantErr && tt.check != nil {
				tt.check(t, result)
			}
		})
	}
}

func TestCustomHandler_Metadata(t *testing.T) {
	handler := NewCustomHandler()
	metadata := handler.Metadata()

	if metadata.Type != "custom" {
		t.Errorf("Expected type 'custom', got %v", metadata.Type)
	}
	if metadata.DisplayName == "" {
		t.Error("Expected non-empty DisplayName")
	}
	if metadata.Description == "" {
		t.Error("Expected non-empty Description")
	}
	if len(metadata.RequiredFields) == 0 {
		t.Error("Expected at least one required field")
	}
}

func TestCustomHandler_BuildPayload(t *testing.T) {
	handler := NewCustomHandler()

	paymentHash := "abc123"
	payment := &models.Payment{
		ID:          "pay_123",
		Amount:      "0.001",
		Currency:    "BTC",
		PaymentHash: &paymentHash,
		PayerInfo: models.JSONMap{
			"email": "test@example.com",
		},
	}

	item := &models.Item{
		ID:   "item_123",
		Name: "Test Product",
	}

	tests := []struct {
		name   string
		config models.JSONMap
		check  func(*testing.T, map[string]interface{})
	}{
		{
			name:   "default payload without template",
			config: models.JSONMap{},
			check: func(t *testing.T, payload map[string]interface{}) {
				if payload["item_id"] != "item_123" {
					t.Errorf("Expected item_id='item_123', got %v", payload["item_id"])
				}
				if payload["payment_id"] != "pay_123" {
					t.Errorf("Expected payment_id='pay_123', got %v", payload["payment_id"])
				}
				if payload["payment_hash"] != "abc123" {
					t.Errorf("Expected payment_hash='abc123', got %v", payload["payment_hash"])
				}
				if payload["payer_email"] != "test@example.com" {
					t.Errorf("Expected payer_email='test@example.com', got %v", payload["payer_email"])
				}
			},
		},
		{
			name: "with custom payload template",
			config: models.JSONMap{
				"payload_template": map[string]interface{}{
					"order_id":   "{item_id}",
					"tx_hash":    "{payment_hash}",
					"amount_btc": "{amount}",
					"currency":   "{currency}",
				},
			},
			check: func(t *testing.T, payload map[string]interface{}) {
				if payload["order_id"] != "item_123" {
					t.Errorf("Expected order_id='item_123', got %v", payload["order_id"])
				}
				if payload["tx_hash"] != "abc123" {
					t.Errorf("Expected tx_hash='abc123', got %v", payload["tx_hash"])
				}
				if payload["amount_btc"] != "0.001" {
					t.Errorf("Expected amount_btc='0.001', got %v", payload["amount_btc"])
				}
				if payload["currency"] != "BTC" {
					t.Errorf("Expected currency='BTC', got %v", payload["currency"])
				}
			},
		},
		{
			name: "nested template expansion",
			config: models.JSONMap{
				"payload_template": map[string]interface{}{
					"transaction": map[string]interface{}{
						"hash":   "{payment_hash}",
						"amount": "{amount}",
					},
					"item": map[string]interface{}{
						"id": "{item_id}",
					},
				},
			},
			check: func(t *testing.T, payload map[string]interface{}) {
				transaction, ok := payload["transaction"].(map[string]interface{})
				if !ok {
					t.Error("Expected transaction to be a map")
					return
				}
				if transaction["hash"] != "abc123" {
					t.Errorf("Expected transaction.hash='abc123', got %v", transaction["hash"])
				}
				if transaction["amount"] != "0.001" {
					t.Errorf("Expected transaction.amount='0.001', got %v", transaction["amount"])
				}

				itemMap, ok := payload["item"].(map[string]interface{})
				if !ok {
					t.Error("Expected item to be a map")
					return
				}
				if itemMap["id"] != "item_123" {
					t.Errorf("Expected item.id='item_123', got %v", itemMap["id"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := handler.buildPayload(payment, item, tt.config)
			if tt.check != nil {
				tt.check(t, payload)
			}
		})
	}
}

func TestCustomHandler_ExpandTemplate(t *testing.T) {
	handler := NewCustomHandler()

	paymentHash := "test_hash_123"
	payment := &models.Payment{
		ID:          "pay_456",
		Amount:      "0.005",
		Currency:    "XMR",
		PaymentHash: &paymentHash,
		PayerInfo: models.JSONMap{
			"email": "user@example.com",
		},
	}

	item := &models.Item{
		ID:   "item_789",
		Name: "Test Item",
	}

	tests := []struct {
		name     string
		input    interface{}
		expected interface{}
	}{
		{
			name:     "expand item_id placeholder",
			input:    "Order for {item_id}",
			expected: "Order for item_789",
		},
		{
			name:     "expand payment_hash placeholder",
			input:    "Transaction: {payment_hash}",
			expected: "Transaction: test_hash_123",
		},
		{
			name:     "expand payment_id placeholder",
			input:    "Payment ID: {payment_id}",
			expected: "Payment ID: pay_456",
		},
		{
			name:     "expand amount placeholder",
			input:    "Amount: {amount}",
			expected: "Amount: 0.005",
		},
		{
			name:     "expand currency placeholder",
			input:    "Currency: {currency}",
			expected: "Currency: XMR",
		},
		{
			name:     "expand payer_email placeholder",
			input:    "Email: {payer_email}",
			expected: "Email: user@example.com",
		},
		{
			name:     "expand multiple placeholders",
			input:    "Order {item_id} paid with {amount} {currency}",
			expected: "Order item_789 paid with 0.005 XMR",
		},
		{
			name:     "no placeholders",
			input:    "static string",
			expected: "static string",
		},
		{
			name:     "non-string value (int)",
			input:    42,
			expected: 42,
		},
		{
			name:     "non-string value (bool)",
			input:    true,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.expandTemplate(tt.input, payment, item)
			if result != tt.expected {
				t.Errorf("expandTemplate() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestCustomHandler_InvokeWebhook(t *testing.T) {
	tests := []struct {
		name     string
		handler  func(w http.ResponseWriter, r *http.Request)
		wantErr  bool
		errMsg   string
		checkRes func(*testing.T, map[string]interface{})
	}{
		{
			name: "successful webhook call",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				response := map[string]interface{}{
					"status":   "fulfilled",
					"order_id": "order_123",
				}
				_ = json.NewEncoder(w).Encode(response)
			},
			wantErr: false,
			checkRes: func(t *testing.T, result map[string]interface{}) {
				if result["status"] != "fulfilled" {
					t.Errorf("Expected status='fulfilled', got %v", result["status"])
				}
				if result["order_id"] != "order_123" {
					t.Errorf("Expected order_id='order_123', got %v", result["order_id"])
				}
			},
		},
		{
			name: "webhook returns error status",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte("Internal server error"))
			},
			wantErr: true,
			errMsg:  "webhook returned status 500",
		},
		{
			name: "webhook returns invalid JSON",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("not valid json"))
			},
			wantErr: true,
			errMsg:  "failed to parse webhook response",
		},
		{
			name: "webhook with custom headers",
			handler: func(w http.ResponseWriter, r *http.Request) {
				// Verify custom header
				if r.Header.Get("X-Custom-Header") != "test-value" {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				response := map[string]interface{}{"status": "ok"}
				_ = json.NewEncoder(w).Encode(response)
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.handler))
			defer server.Close()

			handler := NewCustomHandler()
			ctx := context.Background()

			payload := map[string]interface{}{
				"test": "data",
			}

			config := models.JSONMap{}
			if tt.name == "webhook with custom headers" {
				config["webhook_headers"] = map[string]interface{}{
					"X-Custom-Header": "test-value",
				}
			}

			result, err := handler.invokeWebhook(ctx, server.URL, payload, config)
			if (err != nil) != tt.wantErr {
				t.Errorf("invokeWebhook() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" && err != nil {
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("invokeWebhook() error message = %v, want to contain %v", err.Error(), tt.errMsg)
				}
			}
			if !tt.wantErr && tt.checkRes != nil {
				tt.checkRes(t, result)
			}
		})
	}
}
