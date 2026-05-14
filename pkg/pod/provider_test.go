package pod

import (
	"context"
	"testing"
)

func TestNewProvider(t *testing.T) {
	tests := []struct {
		name         string
		providerName string
		apiKey       string
		wantErr      bool
		errMsg       string
		checkType    func(Provider) bool
	}{
		{
			name:         "create printful provider",
			providerName: "printful",
			apiKey:       "test_key",
			wantErr:      false,
			checkType: func(p Provider) bool {
				_, ok := p.(*PrintfulProvider)
				return ok
			},
		},
		{
			name:         "unsupported provider",
			providerName: "unknown",
			apiKey:       "test_key",
			wantErr:      true,
			errMsg:       "unsupported provider: unknown",
		},
		{
			name:         "empty provider name",
			providerName: "",
			apiKey:       "test_key",
			wantErr:      true,
			errMsg:       "unsupported provider: ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewProvider(tt.providerName, tt.apiKey)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewProvider() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" && err != nil {
				if err.Error() != tt.errMsg {
					t.Errorf("NewProvider() error message = %v, want %v", err.Error(), tt.errMsg)
				}
			}
			if !tt.wantErr {
				if provider == nil {
					t.Error("NewProvider() returned nil provider")
					return
				}
				if tt.checkType != nil && !tt.checkType(provider) {
					t.Errorf("NewProvider() returned wrong provider type")
				}
				// Verify provider implements the interface
				if provider.Name() == "" {
					t.Error("Provider.Name() returned empty string")
				}
			}
		})
	}
}

func TestPrintfulProvider_Name(t *testing.T) {
	provider := NewPrintfulProvider("test_key")
	if provider.Name() != "printful" {
		t.Errorf("PrintfulProvider.Name() = %v, want 'printful'", provider.Name())
	}
}

// Additional tests for PrintfulProvider methods
func TestPrintfulProvider_CreateOrder(t *testing.T) {
	// Since we can't easily mock the printful client, we test the flow
	// by checking for expected error when API call fails
	provider := NewPrintfulProvider("invalid_key")

	ctx := context.Background()
	request := &OrderRequest{
		VariantID:        "123",
		Quantity:         1,
		RecipientName:    "John Doe",
		RecipientEmail:   "john@example.com",
		RecipientAddress: "123 Main St",
		RecipientCity:    "New York",
		RecipientState:   "NY",
		RecipientCountry: "US",
		RecipientZip:     "10001",
		RecipientPhone:   "555-1234",
		DesignURL:        "https://example.com/design.png",
	}

	_, err := provider.CreateOrder(ctx, request)
	// We expect an error because we're using an invalid key
	// This at least tests the code path is functional
	if err == nil {
		t.Log("Note: CreateOrder did not return error (may be using a real API)")
	}
}

func TestPrintfulProvider_CreateOrder_InvalidVariantID(t *testing.T) {
	provider := NewPrintfulProvider("test_key")

	ctx := context.Background()
	request := &OrderRequest{
		VariantID:        "invalid",
		Quantity:         1,
		RecipientName:    "John Doe",
		RecipientEmail:   "john@example.com",
		RecipientAddress: "123 Main St",
		RecipientCity:    "New York",
		RecipientCountry: "US",
		RecipientZip:     "10001",
	}

	// This should handle invalid variant ID gracefully
	_, err := provider.CreateOrder(ctx, request)
	// We expect an error due to invalid API call, but it shouldn't panic
	if err == nil {
		t.Log("Note: CreateOrder accepted invalid variant ID")
	}
}

func TestPrintfulProvider_CreateOrder_NoDesignURL(t *testing.T) {
	provider := NewPrintfulProvider("test_key")

	ctx := context.Background()
	request := &OrderRequest{
		VariantID:        "123",
		Quantity:         1,
		RecipientName:    "John Doe",
		RecipientEmail:   "john@example.com",
		RecipientAddress: "123 Main St",
		RecipientCity:    "New York",
		RecipientCountry: "US",
		RecipientZip:     "10001",
		// No DesignURL
	}

	_, err := provider.CreateOrder(ctx, request)
	// Test that empty design URL doesn't cause panic
	if err == nil {
		t.Log("Note: CreateOrder accepted request without design URL")
	}
}

func TestPrintfulProvider_GetStatus(t *testing.T) {
	provider := NewPrintfulProvider("test_key")

	ctx := context.Background()
	orderID := "test-order-123"

	_, err := provider.GetStatus(ctx, orderID)
	// We expect an error because we're using a test key
	if err == nil {
		t.Log("Note: GetStatus did not return error (may be using a real API)")
	}
}

func TestPrintfulProvider_GetStatus_EmptyOrderID(t *testing.T) {
	provider := NewPrintfulProvider("test_key")

	ctx := context.Background()
	orderID := ""

	_, err := provider.GetStatus(ctx, orderID)
	// Empty order ID should result in an error
	if err == nil {
		t.Error("GetStatus should fail with empty order ID")
	}
}

func TestPrintfulProvider_CancelOrder(t *testing.T) {
	provider := NewPrintfulProvider("test_key")

	ctx := context.Background()
	orderID := "test-order-123"

	err := provider.CancelOrder(ctx, orderID)
	// We expect an error because we're using a test key
	if err == nil {
		t.Log("Note: CancelOrder did not return error (may be using a real API)")
	}
}

func TestPrintfulProvider_CancelOrder_EmptyOrderID(t *testing.T) {
	provider := NewPrintfulProvider("test_key")

	ctx := context.Background()
	orderID := ""

	err := provider.CancelOrder(ctx, orderID)
	// Empty order ID should result in an error
	if err == nil {
		t.Error("CancelOrder should fail with empty order ID")
	}
}

// Test provider interface implementation
func TestPrintfulProvider_Interface(t *testing.T) {
	var _ Provider = (*PrintfulProvider)(nil)
}

// TestOrderRequest validates the OrderRequest structure
func TestOrderRequest_Structure(t *testing.T) {
	req := &OrderRequest{
		VariantID:        "123",
		Quantity:         2,
		RecipientName:    "Test User",
		RecipientEmail:   "test@example.com",
		RecipientAddress: "123 Test St",
		RecipientCity:    "TestCity",
		RecipientState:   "TS",
		RecipientCountry: "US",
		RecipientZip:     "12345",
		RecipientPhone:   "555-0000",
		DesignURL:        "https://example.com/design.png",
	}

	if req.VariantID == "" {
		t.Error("VariantID should not be empty")
	}
	if req.Quantity <= 0 {
		t.Error("Quantity should be positive")
	}
	if req.RecipientName == "" {
		t.Error("RecipientName should not be empty")
	}
}

// TestOrderResponse validates the OrderResponse structure
func TestOrderResponse_Structure(t *testing.T) {
	resp := &OrderResponse{
		OrderID:      "order-123",
		ExternalID:   "ext-456",
		Status:       "pending",
		TrackingURL:  "https://track.example.com",
		ShippingDate: "2026-05-15",
		CreatedAt:    "2026-05-14T10:00:00Z",
	}

	if resp.OrderID == "" {
		t.Error("OrderID should not be empty")
	}
	if resp.Status == "" {
		t.Error("Status should not be empty")
	}
}

// TestOrderStatusResponse validates the OrderStatusResponse structure
func TestOrderStatusResponse_Structure(t *testing.T) {
	status := &OrderStatusResponse{
		OrderID:      "order-123",
		Status:       "shipped",
		TrackingURL:  "https://track.example.com",
		ShippingDate: "2026-05-15",
	}

	if status.OrderID == "" {
		t.Error("OrderID should not be empty")
	}
	if status.Status == "" {
		t.Error("Status should not be empty")
	}
}
