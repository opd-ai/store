package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/opd-ai/store/pkg/models"
)

func TestShippingFormHandler_Validate(t *testing.T) {
	handler := NewShippingFormHandler()

	tests := []struct {
		name    string
		config  models.JSONMap
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config with form_fields",
			config: models.JSONMap{
				"form_fields": map[string]interface{}{
					"address1": map[string]interface{}{
						"label":    "Street Address",
						"required": true,
					},
					"city": map[string]interface{}{
						"label":    "City",
						"required": true,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid config with optional fields",
			config: models.JSONMap{
				"form_fields": map[string]interface{}{
					"address1": map[string]interface{}{
						"label":    "Street Address",
						"required": true,
					},
				},
				"require_phone":        true,
				"require_notes":        false,
				"form_timeout_minutes": 120,
			},
			wantErr: false,
		},
		{
			name:    "missing form_fields",
			config:  models.JSONMap{},
			wantErr: true,
			errMsg:  "missing required field: form_fields",
		},
		{
			name: "missing form_fields with other config",
			config: models.JSONMap{
				"require_phone": true,
			},
			wantErr: true,
			errMsg:  "missing required field: form_fields",
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

func TestShippingFormHandler_Handle(t *testing.T) {
	handler := NewShippingFormHandler()
	ctx := context.Background()

	tests := []struct {
		name    string
		payment *models.Payment
		item    *models.Item
		wantErr bool
		errMsg  string
		check   func(*testing.T, map[string]interface{})
	}{
		{
			name: "successful form generation for confirmed payment",
			payment: &models.Payment{
				ID:     "pay_123",
				Status: "confirmed",
			},
			item: &models.Item{
				ID:   "item_123",
				Name: "Physical Product",
				BackendConfig: models.JSONMap{
					"form_fields": map[string]interface{}{
						"address1": map[string]interface{}{
							"label":    "Street Address",
							"required": true,
						},
					},
				},
			},
			wantErr: false,
			check: func(t *testing.T, result map[string]interface{}) {
				if result["form_url"] == nil {
					t.Error("Expected form_url in result")
				}
				if result["status"] != "awaiting_address" {
					t.Errorf("Expected status='awaiting_address', got %v", result["status"])
				}
				if result["payment_id"] != "pay_123" {
					t.Errorf("Expected payment_id='pay_123', got %v", result["payment_id"])
				}
				if result["timeout_minutes"] == nil {
					t.Error("Expected timeout_minutes in result")
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
				Name: "Physical Product",
				BackendConfig: models.JSONMap{
					"form_fields": map[string]interface{}{
						"address1": map[string]interface{}{
							"label":    "Street Address",
							"required": true,
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "payment not confirmed",
		},
		{
			name: "payment cancelled",
			payment: &models.Payment{
				ID:     "pay_123",
				Status: "cancelled",
			},
			item: &models.Item{
				ID:   "item_123",
				Name: "Physical Product",
				BackendConfig: models.JSONMap{
					"form_fields": map[string]interface{}{
						"address1": map[string]interface{}{
							"label":    "Street Address",
							"required": true,
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "payment not confirmed",
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

func TestShippingFormHandler_Metadata(t *testing.T) {
	handler := NewShippingFormHandler()
	metadata := handler.Metadata()

	if metadata.Type != "shipping_form" {
		t.Errorf("Expected type 'shipping_form', got %v", metadata.Type)
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

func TestValidateFormData(t *testing.T) {
	tests := []struct {
		name    string
		data    FormData
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid form data",
			data: FormData{
				Address1:    "123 Main St",
				City:        "San Francisco",
				State:       "CA",
				PostalCode:  "94102",
				Country:     "US",
				Phone:       "+1-555-0123",
				SubmittedAt: time.Now(),
			},
			wantErr: false,
		},
		{
			name: "valid form data with optional fields",
			data: FormData{
				Address1:    "123 Main St",
				Address2:    "Apt 4B",
				City:        "San Francisco",
				State:       "CA",
				PostalCode:  "94102",
				Country:     "US",
				Phone:       "+1-555-0123",
				Notes:       "Leave at door",
				SubmittedAt: time.Now(),
			},
			wantErr: false,
		},
		{
			name: "missing address1",
			data: FormData{
				City:        "San Francisco",
				State:       "CA",
				PostalCode:  "94102",
				Country:     "US",
				SubmittedAt: time.Now(),
			},
			wantErr: true,
			errMsg:  "address1 is required",
		},
		{
			name: "missing city",
			data: FormData{
				Address1:    "123 Main St",
				State:       "CA",
				PostalCode:  "94102",
				Country:     "US",
				SubmittedAt: time.Now(),
			},
			wantErr: true,
			errMsg:  "city is required",
		},
		{
			name: "missing state",
			data: FormData{
				Address1:    "123 Main St",
				City:        "San Francisco",
				PostalCode:  "94102",
				Country:     "US",
				SubmittedAt: time.Now(),
			},
			wantErr: true,
			errMsg:  "state is required",
		},
		{
			name: "missing postal_code",
			data: FormData{
				Address1:    "123 Main St",
				City:        "San Francisco",
				State:       "CA",
				Country:     "US",
				SubmittedAt: time.Now(),
			},
			wantErr: true,
			errMsg:  "postal_code is required",
		},
		{
			name: "missing country",
			data: FormData{
				Address1:    "123 Main St",
				City:        "San Francisco",
				State:       "CA",
				PostalCode:  "94102",
				SubmittedAt: time.Now(),
			},
			wantErr: true,
			errMsg:  "country is required",
		},
		{
			name: "all fields empty",
			data: FormData{
				SubmittedAt: time.Now(),
			},
			wantErr: true,
			errMsg:  "address1 is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFormData(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateFormData() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" && err != nil {
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("ValidateFormData() error message = %v, want to contain %v", err.Error(), tt.errMsg)
				}
			}
		})
	}
}
