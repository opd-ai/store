package store_test

import (
	"context"
	"testing"

	"github.com/opd-ai/store/pkg/models"
)

// TestDetermineValidationType tests the DetermineValidationType function behavior.
func TestDetermineValidationType(t *testing.T) {
	t.Run("returns provided type when hasType is true", func(t *testing.T) {
		s := setupTestStore(t)
		ctx := context.Background()

		// This should use the provided type without looking up the item
		validationType, err := s.DetermineValidationType(ctx, "nonexistent-item", "digital_media", true)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		if validationType != "digital_media" {
			t.Errorf("expected 'digital_media', got %s", validationType)
		}
	})

	t.Run("fetches type from existing item when hasType is false", func(t *testing.T) {
		s := setupTestStore(t)
		ctx := context.Background()

		// Create a category first
		cat, err := s.CreateCategory(ctx, "Test Category", "Test")
		if err != nil {
			t.Fatalf("failed to create category: %v", err)
		}

		// Create an item using NewItem (which sets ID properly)
		item := models.NewItem(cat.ID, "Test Item", "Test Description", "100000", "BTC", "shipping_form")
		item.BackendConfig = models.JSONMap{
			"address_required": true,
			"form_fields": []interface{}{
				map[string]interface{}{
					"name":     "address",
					"label":    "Shipping Address",
					"type":     "text",
					"required": true,
				},
			},
		}
		createdItem, err := s.CreateItem(ctx, item)
		if err != nil {
			t.Fatalf("failed to create item: %v", err)
		}

		// Fetch type from item
		validationType, err := s.DetermineValidationType(ctx, createdItem.ID, "", false)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		if validationType != "shipping_form" {
			t.Errorf("expected 'shipping_form', got %s", validationType)
		}
	})

	t.Run("returns error when item not found and hasType is false", func(t *testing.T) {
		s := setupTestStore(t)
		ctx := context.Background()

		_, err := s.DetermineValidationType(ctx, "nonexistent-item", "", false)
		if err == nil {
			t.Fatal("expected error for nonexistent item, got nil")
		}
	})
}

// TestDetermineConfigToValidate tests the DetermineConfigToValidate function behavior.
func TestDetermineConfigToValidate(t *testing.T) {
	t.Run("returns provided config when hasConfig is true", func(t *testing.T) {
		s := setupTestStore(t)
		ctx := context.Background()

		providedConfig := models.JSONMap{
			"file_path": "/test/path",
			"storage":   "local",
		}

		config, err := s.DetermineConfigToValidate(ctx, "nonexistent-item", providedConfig, true)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		if config["file_path"] != "/test/path" {
			t.Errorf("expected config to match provided config")
		}
		if config["storage"] != "local" {
			t.Errorf("expected storage to be 'local', got %v", config["storage"])
		}
	})

	t.Run("fetches config from existing item when hasConfig is false", func(t *testing.T) {
		s := setupTestStore(t)
		ctx := context.Background()

		// Create a category first
		cat, err := s.CreateCategory(ctx, "Test Category", "Test")
		if err != nil {
			t.Fatalf("failed to create category: %v", err)
		}

		// Create an item using NewItem
		item := models.NewItem(cat.ID, "Test Item", "Test Description", "100000", "BTC", "digital_media")
		item.BackendConfig = models.JSONMap{
			"file_path":        "/existing/path",
			"storage":          "local",
			"expiration_hours": 24.0,
		}
		createdItem, err := s.CreateItem(ctx, item)
		if err != nil {
			t.Fatalf("failed to create item: %v", err)
		}

		// Fetch config from item
		config, err := s.DetermineConfigToValidate(ctx, createdItem.ID, nil, false)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		if config["file_path"] != "/existing/path" {
			t.Errorf("expected file_path to be '/existing/path', got %v", config["file_path"])
		}
		if config["storage"] != "local" {
			t.Errorf("expected storage to be 'local', got %v", config["storage"])
		}
	})

	t.Run("returns error when item not found and hasConfig is false", func(t *testing.T) {
		s := setupTestStore(t)
		ctx := context.Background()

		_, err := s.DetermineConfigToValidate(ctx, "nonexistent-item", nil, false)
		if err == nil {
			t.Fatal("expected error for nonexistent item, got nil")
		}
	})

	t.Run("returns empty config when hasConfig is true but config is nil", func(t *testing.T) {
		s := setupTestStore(t)
		ctx := context.Background()

		config, err := s.DetermineConfigToValidate(ctx, "any-item", nil, true)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		if config != nil {
			t.Errorf("expected nil config, got %v", config)
		}
	})
}

// TestValidateBackendUpdate tests validation during item updates.
func TestValidateBackendUpdate(t *testing.T) {
	t.Run("validates type change with new config", func(t *testing.T) {
		s := setupTestStore(t)
		ctx := context.Background()

		// Create a category
		cat, err := s.CreateCategory(ctx, "Test Category", "Test")
		if err != nil {
			t.Fatalf("failed to create category: %v", err)
		}

		// Create an item
		item := models.NewItem(cat.ID, "Test Item", "Test Description", "100000", "BTC", "digital_media")
		item.BackendConfig = models.JSONMap{
			"file_path":        "/test/file.pdf",
			"storage":          "local",
			"expiration_hours": 24.0,
		}
		createdItem, err := s.CreateItem(ctx, item)
		if err != nil {
			t.Fatalf("failed to create item: %v", err)
		}

		// Update to shipping_form type with valid config
		err = s.UpdateItem(ctx, createdItem.ID, map[string]interface{}{
			"backend_type": "shipping_form",
			"backend_config": models.JSONMap{
				"address_required": true,
				"form_fields": []interface{}{
					map[string]interface{}{
						"name":     "address",
						"label":    "Address",
						"type":     "text",
						"required": true,
					},
				},
			},
		})
		if err != nil {
			t.Errorf("expected valid update, got error: %v", err)
		}
	})

	t.Run("rejects invalid config for handler type", func(t *testing.T) {
		s := setupTestStore(t)
		ctx := context.Background()

		// Create a category
		cat, err := s.CreateCategory(ctx, "Test Category", "Test")
		if err != nil {
			t.Fatalf("failed to create category: %v", err)
		}

		// Create an item
		item := models.NewItem(cat.ID, "Test Item", "Test Description", "100000", "BTC", "digital_media")
		item.BackendConfig = models.JSONMap{
			"file_path":        "/test/file.pdf",
			"storage":          "local",
			"expiration_hours": 24.0,
		}
		createdItem, err := s.CreateItem(ctx, item)
		if err != nil {
			t.Fatalf("failed to create item: %v", err)
		}

		// Try to update with invalid config (missing file_path for local storage)
		err = s.UpdateItem(ctx, createdItem.ID, map[string]interface{}{
			"backend_config": models.JSONMap{
				"storage":          "local",
				"expiration_hours": 24.0,
				// Missing file_path - should fail validation
			},
		})
		if err == nil {
			t.Error("expected validation error for missing file_path, got nil")
		}
	})

	t.Run("allows update without backend changes", func(t *testing.T) {
		s := setupTestStore(t)
		ctx := context.Background()

		// Create a category
		cat, err := s.CreateCategory(ctx, "Test Category", "Test")
		if err != nil {
			t.Fatalf("failed to create category: %v", err)
		}

		// Create an item
		item := models.NewItem(cat.ID, "Test Item", "Test Description", "100000", "BTC", "digital_media")
		item.BackendConfig = models.JSONMap{
			"file_path":        "/test/file.pdf",
			"storage":          "local",
			"expiration_hours": 24.0,
		}
		createdItem, err := s.CreateItem(ctx, item)
		if err != nil {
			t.Fatalf("failed to create item: %v", err)
		}

		// Update non-backend fields
		err = s.UpdateItem(ctx, createdItem.ID, map[string]interface{}{
			"name":        "Updated Name",
			"description": "Updated Description",
			"price":       "200000",
		})
		if err != nil {
			t.Errorf("expected valid update, got error: %v", err)
		}
	})

	t.Run("rejects unregistered handler type", func(t *testing.T) {
		s := setupTestStore(t)
		ctx := context.Background()

		// Create a category
		cat, err := s.CreateCategory(ctx, "Test Category", "Test")
		if err != nil {
			t.Fatalf("failed to create category: %v", err)
		}

		// Create an item
		item := models.NewItem(cat.ID, "Test Item", "Test Description", "100000", "BTC", "digital_media")
		item.BackendConfig = models.JSONMap{
			"file_path":        "/test/file.pdf",
			"storage":          "local",
			"expiration_hours": 24.0,
		}
		createdItem, err := s.CreateItem(ctx, item)
		if err != nil {
			t.Fatalf("failed to create item: %v", err)
		}

		// Try to update with unregistered handler type
		err = s.UpdateItem(ctx, createdItem.ID, map[string]interface{}{
			"backend_type": "nonexistent_handler",
		})
		if err == nil {
			t.Error("expected error for unregistered handler type, got nil")
		}
	})
}
