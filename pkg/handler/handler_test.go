package handler_test

import (
	"testing"

	"github.com/opd-ai/store/internal/handlers"
	"github.com/opd-ai/store/pkg/handler"
)

// TestHandlerRegistry tests handler registration and lookup.
func TestHandlerRegistry(t *testing.T) {
	reg := handler.NewRegistry()

	// Register handlers
	handlersToRegister := []handler.FulfillmentHandler{
		handlers.NewDigitalMediaHandler(),
		handlers.NewShippingFormHandler(),
		handlers.NewPrintOnDemandHandler(),
		handlers.NewCustomHandler(),
	}

	for _, h := range handlersToRegister {
		if err := reg.Register(h); err != nil {
			t.Errorf("failed to register handler: %v", err)
		}
	}

	// Verify all handlers are registered
	all := reg.All()
	if len(all) != 4 {
		t.Errorf("expected 4 handlers, got %d", len(all))
	}

	// Test lookup
	digitalHandler, err := reg.Get("digital_media")
	if err != nil {
		t.Errorf("failed to get digital_media handler: %v", err)
	}
	if digitalHandler == nil {
		t.Error("expected handler, got nil")
	}

	// Test lookup of non-existent handler
	_, err = reg.Get("nonexistent")
	if err == nil {
		t.Error("expected error for non-existent handler")
	}
}

// TestRegistryDuplicateRegistration tests that duplicate handler registration fails.
func TestRegistryDuplicateRegistration(t *testing.T) {
	reg := handler.NewRegistry()
	h := handlers.NewDigitalMediaHandler()

	// First registration should succeed
	if err := reg.Register(h); err != nil {
		t.Errorf("first registration failed: %v", err)
	}

	// Second registration should fail
	if err := reg.Register(h); err == nil {
		t.Error("expected error for duplicate registration")
	}
}

// TestConfigGetString tests the Config.GetString method.
func TestConfigGetString(t *testing.T) {
	tests := []struct {
		name     string
		config   handler.Config
		key      string
		expected string
	}{
		{
			name: "Valid string value",
			config: handler.Config{
				Settings: map[string]interface{}{
					"name": "test-value",
				},
			},
			key:      "name",
			expected: "test-value",
		},
		{
			name: "Missing key",
			config: handler.Config{
				Settings: map[string]interface{}{},
			},
			key:      "nonexistent",
			expected: "",
		},
		{
			name: "Non-string value",
			config: handler.Config{
				Settings: map[string]interface{}{
					"number": 123,
				},
			},
			key:      "number",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetString(tt.key)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// TestConfigGetInt tests the Config.GetInt method.
func TestConfigGetInt(t *testing.T) {
	tests := []struct {
		name     string
		config   handler.Config
		key      string
		expected int
	}{
		{
			name: "Valid int value",
			config: handler.Config{
				Settings: map[string]interface{}{
					"count": 42,
				},
			},
			key:      "count",
			expected: 42,
		},
		{
			name: "Valid float64 value",
			config: handler.Config{
				Settings: map[string]interface{}{
					"count": float64(99),
				},
			},
			key:      "count",
			expected: 99,
		},
		{
			name: "Missing key",
			config: handler.Config{
				Settings: map[string]interface{}{},
			},
			key:      "nonexistent",
			expected: 0,
		},
		{
			name: "Non-numeric value",
			config: handler.Config{
				Settings: map[string]interface{}{
					"text": "not a number",
				},
			},
			key:      "text",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetInt(tt.key)
			if result != tt.expected {
				t.Errorf("Expected %d, got %d", tt.expected, result)
			}
		})
	}
}

// TestConfigGetBool tests the Config.GetBool method.
func TestConfigGetBool(t *testing.T) {
	tests := []struct {
		name     string
		config   handler.Config
		key      string
		expected bool
	}{
		{
			name: "Valid true value",
			config: handler.Config{
				Settings: map[string]interface{}{
					"enabled": true,
				},
			},
			key:      "enabled",
			expected: true,
		},
		{
			name: "Valid false value",
			config: handler.Config{
				Settings: map[string]interface{}{
					"enabled": false,
				},
			},
			key:      "enabled",
			expected: false,
		},
		{
			name: "Missing key",
			config: handler.Config{
				Settings: map[string]interface{}{},
			},
			key:      "nonexistent",
			expected: false,
		},
		{
			name: "Non-boolean value",
			config: handler.Config{
				Settings: map[string]interface{}{
					"text": "yes",
				},
			},
			key:      "text",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetBool(tt.key)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}
