package handlers

import (
	"context"
	"testing"

	"github.com/opd-ai/store/pkg/models"
)

func TestPrintOnDemandHandler_Validate(t *testing.T) {
	handler := NewPrintOnDemandHandler()

	tests := []struct {
		name    string
		config  models.JSONMap
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid printful config",
			config: models.JSONMap{
				"provider": "printful",
				"api_key":  "test_key",
				"product_mapping": map[string]interface{}{
					"item-123": map[string]interface{}{
						"product_id": 456,
						"variant_id": 789,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "missing provider",
			config: models.JSONMap{
				"api_key": "test_key",
				"product_mapping": map[string]interface{}{
					"item-123": map[string]interface{}{
						"variant_id": 789,
					},
				},
			},
			wantErr: true,
			errMsg:  "missing required field: provider",
		},
		{
			name: "missing api_key",
			config: models.JSONMap{
				"provider": "printful",
				"product_mapping": map[string]interface{}{
					"item-123": map[string]interface{}{
						"variant_id": 789,
					},
				},
			},
			wantErr: true,
			errMsg:  "missing required field: api_key",
		},
		{
			name: "missing product_mapping",
			config: models.JSONMap{
				"provider": "printful",
				"api_key":  "test_key",
			},
			wantErr: true,
			errMsg:  "missing required field: product_mapping",
		},
		{
			name: "invalid provider type (not string)",
			config: models.JSONMap{
				"provider": 123,
				"api_key":  "test_key",
				"product_mapping": map[string]interface{}{
					"item-123": map[string]interface{}{
						"variant_id": 789,
					},
				},
			},
			wantErr: true,
			errMsg:  "invalid provider type",
		},
		{
			name: "unsupported provider",
			config: models.JSONMap{
				"provider": "unknown_provider",
				"api_key":  "test_key",
				"product_mapping": map[string]interface{}{
					"item-123": map[string]interface{}{
						"variant_id": 789,
					},
				},
			},
			wantErr: true,
			errMsg:  "unsupported provider: unknown_provider",
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

func TestPrintOnDemandHandler_Handle(t *testing.T) {
	handler := NewPrintOnDemandHandler()
	ctx := context.Background()

	tests := []struct {
		name    string
		payment *models.Payment
		item    *models.Item
		wantErr bool
		errMsg  string
		check   func(*testing.T, map[string]interface{})
	}{
		// Note: Removed "successful order creation" test as it requires real API credentials
		// In a real implementation, this would be tested with mocked providers
		{
			name: "payment not confirmed",
			payment: &models.Payment{
				ID:     "pay_123",
				Status: "pending",
				PayerInfo: models.JSONMap{
					"name":         "John Doe",
					"address1":     "123 Main St",
					"city":         "San Francisco",
					"state_code":   "CA",
					"country_code": "US",
					"zip":          "94102",
				},
			},
			item: &models.Item{
				ID:   "item_123",
				Name: "T-Shirt",
				BackendConfig: models.JSONMap{
					"provider": "printful",
					"api_key":  "test_key",
					"product_mapping": map[string]interface{}{
						"item_123": map[string]interface{}{
							"variant_id": "1234",
						},
					},
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
				Name:          "T-Shirt",
				BackendConfig: nil,
			},
			wantErr: true,
			errMsg:  "missing backend configuration",
		},
		{
			name: "missing provider in config",
			payment: &models.Payment{
				ID:     "pay_123",
				Status: "confirmed",
			},
			item: &models.Item{
				ID:   "item_123",
				Name: "T-Shirt",
				BackendConfig: models.JSONMap{
					"api_key": "test_key",
				},
			},
			wantErr: true,
			errMsg:  "missing or invalid provider",
		},
		{
			name: "missing api_key in config",
			payment: &models.Payment{
				ID:     "pay_123",
				Status: "confirmed",
			},
			item: &models.Item{
				ID:   "item_123",
				Name: "T-Shirt",
				BackendConfig: models.JSONMap{
					"provider": "printful",
				},
			},
			wantErr: true,
			errMsg:  "missing or invalid api_key",
		},
		{
			name: "missing product_mapping in config",
			payment: &models.Payment{
				ID:     "pay_123",
				Status: "confirmed",
			},
			item: &models.Item{
				ID:   "item_123",
				Name: "T-Shirt",
				BackendConfig: models.JSONMap{
					"provider": "printful",
					"api_key":  "test_key",
				},
			},
			wantErr: true,
			errMsg:  "missing or invalid product_mapping",
		},
		{
			name: "missing payer info",
			payment: &models.Payment{
				ID:        "pay_123",
				Status:    "confirmed",
				PayerInfo: nil,
			},
			item: &models.Item{
				ID:   "item_123",
				Name: "T-Shirt",
				BackendConfig: models.JSONMap{
					"provider": "printful",
					"api_key":  "test_key",
					"product_mapping": map[string]interface{}{
						"item_123": map[string]interface{}{
							"variant_id": "1234",
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "payer info is nil",
		},
		{
			name: "incomplete payer info (missing required fields)",
			payment: &models.Payment{
				ID:     "pay_123",
				Status: "confirmed",
				PayerInfo: models.JSONMap{
					"name": "John Doe",
					// Missing address, city, zip, country
				},
			},
			item: &models.Item{
				ID:   "item_123",
				Name: "T-Shirt",
				BackendConfig: models.JSONMap{
					"provider": "printful",
					"api_key":  "test_key",
					"product_mapping": map[string]interface{}{
						"item_123": map[string]interface{}{
							"variant_id": "1234",
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "missing required shipping information",
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

func TestPrintOnDemandHandler_Metadata(t *testing.T) {
	handler := NewPrintOnDemandHandler()
	metadata := handler.Metadata()

	if metadata.Type != "pod" {
		t.Errorf("Expected type 'pod', got %v", metadata.Type)
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

func TestExtractRecipientFromPayerInfo(t *testing.T) {
	tests := []struct {
		name      string
		payerInfo models.JSONMap
		wantErr   bool
		errMsg    string
		check     func(*testing.T, *recipientInfo)
	}{
		{
			name: "valid payer info with all fields",
			payerInfo: models.JSONMap{
				"name":         "John Doe",
				"address1":     "123 Main St",
				"address2":     "Apt 4B",
				"city":         "San Francisco",
				"state_code":   "CA",
				"country_code": "US",
				"zip":          "94102",
				"email":        "john@example.com",
				"phone":        "+1-555-0123",
			},
			wantErr: false,
			check: func(t *testing.T, info *recipientInfo) {
				if info.Name != "John Doe" {
					t.Errorf("Expected name='John Doe', got %v", info.Name)
				}
				if !contains(info.Address, "123 Main St") {
					t.Errorf("Expected address to contain '123 Main St', got %v", info.Address)
				}
				if !contains(info.Address, "Apt 4B") {
					t.Errorf("Expected address to contain 'Apt 4B', got %v", info.Address)
				}
				if info.City != "San Francisco" {
					t.Errorf("Expected city='San Francisco', got %v", info.City)
				}
			},
		},
		{
			name: "valid payer info without address2",
			payerInfo: models.JSONMap{
				"name":         "Jane Smith",
				"address1":     "456 Oak Ave",
				"city":         "New York",
				"state_code":   "NY",
				"country_code": "US",
				"zip":          "10001",
			},
			wantErr: false,
			check: func(t *testing.T, info *recipientInfo) {
				if info.Name != "Jane Smith" {
					t.Errorf("Expected name='Jane Smith', got %v", info.Name)
				}
				if info.Address != "456 Oak Ave" {
					t.Errorf("Expected address='456 Oak Ave', got %v", info.Address)
				}
			},
		},
		{
			name:      "nil payer info",
			payerInfo: nil,
			wantErr:   true,
			errMsg:    "payer info is nil",
		},
		{
			name: "missing name",
			payerInfo: models.JSONMap{
				"address1":     "123 Main St",
				"city":         "San Francisco",
				"country_code": "US",
				"zip":          "94102",
			},
			wantErr: true,
			errMsg:  "missing required shipping information",
		},
		{
			name: "missing address1",
			payerInfo: models.JSONMap{
				"name":         "John Doe",
				"city":         "San Francisco",
				"country_code": "US",
				"zip":          "94102",
			},
			wantErr: true,
			errMsg:  "missing required shipping information",
		},
		{
			name: "missing city",
			payerInfo: models.JSONMap{
				"name":         "John Doe",
				"address1":     "123 Main St",
				"country_code": "US",
				"zip":          "94102",
			},
			wantErr: true,
			errMsg:  "missing required shipping information",
		},
		{
			name: "missing country_code",
			payerInfo: models.JSONMap{
				"name":     "John Doe",
				"address1": "123 Main St",
				"city":     "San Francisco",
				"zip":      "94102",
			},
			wantErr: true,
			errMsg:  "missing required shipping information",
		},
		{
			name: "missing zip",
			payerInfo: models.JSONMap{
				"name":         "John Doe",
				"address1":     "123 Main St",
				"city":         "San Francisco",
				"country_code": "US",
			},
			wantErr: true,
			errMsg:  "missing required shipping information",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := extractRecipientFromPayerInfo(tt.payerInfo)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractRecipientFromPayerInfo() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" && err != nil {
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("extractRecipientFromPayerInfo() error message = %v, want to contain %v", err.Error(), tt.errMsg)
				}
			}
			if !tt.wantErr && tt.check != nil {
				tt.check(t, result)
			}
		})
	}
}
