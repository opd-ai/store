package handlers

import (
	"context"
	"testing"

	"github.com/opd-ai/store/pkg/models"
)

func TestDigitalMediaHandler_Validate(t *testing.T) {
	handler := NewDigitalMediaHandler()

	tests := []struct {
		name    string
		config  models.JSONMap
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid local storage config",
			config: models.JSONMap{
				"storage":          "local",
				"file_path":        "/downloads/product.pdf",
				"expiration_hours": 24,
			},
			wantErr: false,
		},
		{
			name: "valid s3 storage config",
			config: models.JSONMap{
				"storage":          "s3",
				"s3_bucket":        "my-bucket",
				"s3_region":        "us-east-1",
				"s3_key":           "products/file.pdf",
				"expiration_hours": 48,
			},
			wantErr: false,
		},
		{
			name: "local storage missing file_path",
			config: models.JSONMap{
				"storage":          "local",
				"expiration_hours": 24,
			},
			wantErr: true,
			errMsg:  "file_path is required",
		},
		{
			name: "s3 storage missing bucket",
			config: models.JSONMap{
				"storage":          "s3",
				"s3_region":        "us-east-1",
				"expiration_hours": 24,
			},
			wantErr: true,
			errMsg:  "s3_bucket is required",
		},
		{
			name: "s3 storage missing region",
			config: models.JSONMap{
				"storage":          "s3",
				"s3_bucket":        "my-bucket",
				"expiration_hours": 24,
			},
			wantErr: true,
			errMsg:  "s3_region is required",
		},
		{
			name: "invalid storage type",
			config: models.JSONMap{
				"storage":          "azure",
				"expiration_hours": 24,
			},
			wantErr: true,
			errMsg:  "unsupported storage type",
		},
		{
			name: "invalid expiration hours",
			config: models.JSONMap{
				"storage":          "local",
				"file_path":        "/downloads/product.pdf",
				"expiration_hours": 0,
			},
			wantErr: true,
			errMsg:  "expiration_hours must be at least 1",
		},
		{
			name: "negative expiration hours",
			config: models.JSONMap{
				"storage":          "local",
				"file_path":        "/downloads/product.pdf",
				"expiration_hours": -5,
			},
			wantErr: true,
			errMsg:  "expiration_hours must be at least 1",
		},
		{
			name: "default storage (empty) with file_path",
			config: models.JSONMap{
				"file_path":        "/downloads/product.pdf",
				"expiration_hours": 24,
			},
			wantErr: false, // defaults to local
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
				if err.Error() != tt.errMsg && !contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error message = %v, want to contain %v", err.Error(), tt.errMsg)
				}
			}
		})
	}
}

func TestDigitalMediaHandler_Handle(t *testing.T) {
	handler := NewDigitalMediaHandler()
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
			name: "successful local storage fulfillment",
			payment: &models.Payment{
				ID:     "pay_123",
				Status: "confirmed",
			},
			item: &models.Item{
				ID:   "item_123",
				Name: "Test Product",
				BackendConfig: models.JSONMap{
					"storage":          "local",
					"file_path":        "/downloads/product.pdf",
					"expiration_hours": 24,
					"max_downloads":    5,
				},
			},
			wantErr: false,
			check: func(t *testing.T, result map[string]interface{}) {
				if result["download_url"] == nil {
					t.Error("Expected download_url in result")
				}
				if result["expires_at"] == nil {
					t.Error("Expected expires_at in result")
				}
				if result["max_downloads"] != 5 {
					t.Errorf("Expected max_downloads=5, got %v", result["max_downloads"])
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
					"storage":          "local",
					"file_path":        "/downloads/product.pdf",
					"expiration_hours": 24,
				},
			},
			wantErr: true,
			errMsg:  "payment not confirmed",
		},
		{
			name: "local storage missing file_path in config",
			payment: &models.Payment{
				ID:     "pay_123",
				Status: "confirmed",
			},
			item: &models.Item{
				ID:   "item_123",
				Name: "Test Product",
				BackendConfig: models.JSONMap{
					"storage":          "local",
					"expiration_hours": 24,
				},
			},
			wantErr: true,
			errMsg:  "file_path not configured",
		},
		{
			name: "default expiration when not specified",
			payment: &models.Payment{
				ID:     "pay_123",
				Status: "confirmed",
			},
			item: &models.Item{
				ID:   "item_123",
				Name: "Test Product",
				BackendConfig: models.JSONMap{
					"storage":   "local",
					"file_path": "/downloads/product.pdf",
				},
			},
			wantErr: false,
			check: func(t *testing.T, result map[string]interface{}) {
				if result["expires_at"] == nil {
					t.Error("Expected expires_at in result with default value")
				}
			},
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

func TestDigitalMediaHandler_Metadata(t *testing.T) {
	handler := NewDigitalMediaHandler()
	metadata := handler.Metadata()

	if metadata.Type != "digital_media" {
		t.Errorf("Expected type 'digital_media', got %v", metadata.Type)
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

// Helper function for substring matching
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
