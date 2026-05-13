package printful

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	client := NewClient("test-api-key")
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.apiKey != "test-api-key" {
		t.Errorf("expected apiKey to be 'test-api-key', got %s", client.apiKey)
	}
	if client.baseURL != "https://api.printful.com" {
		t.Errorf("expected baseURL to be 'https://api.printful.com', got %s", client.baseURL)
	}
}

func TestCreateOrder_Success(t *testing.T) {
	// Create a test server that mocks the Printful API
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST request, got %s", r.Method)
		}
		if r.URL.Path != "/orders" {
			t.Errorf("expected path /orders, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Authorization header with Bearer test-key")
		}

		// Return a successful response
		resp := printfulResponse{
			Code: 200,
			Result: json.RawMessage(`{
				"id": "12345",
				"external_id": "ext-001",
				"status": "draft",
				"created": "2026-05-13T10:00:00Z"
			}`),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient("test-key")
	client.baseURL = server.URL

	orderReq := &OrderRequest{
		Recipient: Recipient{
			Name:        "John Doe",
			Address1:    "123 Main St",
			City:        "New York",
			StateCode:   "NY",
			CountryCode: "US",
			Zip:         "10001",
		},
		Items: []OrderItem{
			{
				VariantID: 4011,
				Quantity:  1,
			},
		},
		ConfirmDraft: false,
	}

	order, err := client.CreateOrder(context.Background(), orderReq)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if order.OrderID != "12345" {
		t.Errorf("expected order ID '12345', got %s", order.OrderID)
	}
	if order.Status != "draft" {
		t.Errorf("expected status 'draft', got %s", order.Status)
	}
}

func TestCreateOrder_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := printfulResponse{
			Code: 400,
			Error: &printfulError{
				Code:    400,
				Message: "Invalid variant ID",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK) // Printful returns 200 even for errors in the response body
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient("test-key")
	client.baseURL = server.URL

	orderReq := &OrderRequest{
		Recipient: Recipient{
			Name:        "John Doe",
			Address1:    "123 Main St",
			City:        "New York",
			CountryCode: "US",
			Zip:         "10001",
		},
		Items: []OrderItem{
			{
				VariantID: 9999999, // Invalid variant
				Quantity:  1,
			},
		},
	}

	_, err := client.CreateOrder(context.Background(), orderReq)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetOrderStatus_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got %s", r.Method)
		}
		if r.URL.Path != "/orders/12345" {
			t.Errorf("expected path /orders/12345, got %s", r.URL.Path)
		}

		resp := printfulResponse{
			Code: 200,
			Result: json.RawMessage(`{
				"id": "12345",
				"status": "fulfilled",
				"tracking_url": "https://tracking.example.com/12345",
				"shipping_date": "2026-05-15"
			}`),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient("test-key")
	client.baseURL = server.URL

	status, err := client.GetOrderStatus(context.Background(), "12345")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.OrderID != "12345" {
		t.Errorf("expected order ID '12345', got %s", status.OrderID)
	}
	if status.Status != "fulfilled" {
		t.Errorf("expected status 'fulfilled', got %s", status.Status)
	}
	if status.TrackingURL != "https://tracking.example.com/12345" {
		t.Errorf("expected tracking URL, got %s", status.TrackingURL)
	}
}

func TestCancelOrder_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE request, got %s", r.Method)
		}
		if r.URL.Path != "/orders/12345" {
			t.Errorf("expected path /orders/12345, got %s", r.URL.Path)
		}

		resp := printfulResponse{
			Code:   200,
			Result: json.RawMessage(`{"id": "12345", "status": "canceled"}`),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient("test-key")
	client.baseURL = server.URL

	err := client.CancelOrder(context.Background(), "12345")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateOrder_ContextCanceled(t *testing.T) {
	// Create a server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		resp := printfulResponse{Code: 200}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient("test-key")
	client.baseURL = server.URL

	// Create a context that's already canceled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	orderReq := &OrderRequest{
		Recipient: Recipient{
			Name:        "John Doe",
			CountryCode: "US",
		},
		Items: []OrderItem{{VariantID: 4011, Quantity: 1}},
	}

	_, err := client.CreateOrder(ctx, orderReq)
	if err == nil {
		t.Fatal("expected error from canceled context, got nil")
	}
}
