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
