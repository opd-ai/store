// Package handler provides the fulfillment handler interface and registry.
// It defines the contract that all fulfillment backends must implement
// (digital media, shipping forms, print-on-demand, custom webhooks).
//
// Key types: FulfillmentHandler, HandlerRegistry.
//
// Example usage:
//
//	registry := handler.NewRegistry()
//	registry.Register("digital_media", digitalMediaHandler)
//	handler, _ := registry.Get("digital_media")
package handler

import (
	"context"
	"fmt"
	"sync"

	"github.com/opd-ai/store/pkg/models"
)

// FulfillmentHandler defines the contract for post-payment fulfillment logic.
// Implementations must handle payment verification and execute backend-specific actions.
type FulfillmentHandler interface {
	// Handle processes the payment and executes backend-specific fulfillment.
	//
	// Parameters:
	//   - ctx: context for cancellation and timeouts
	//   - payment: Payment object with verified payment details
	//   - item: Item metadata with backend-specific configuration
	//
	// Returns:
	//   - result: map[string]interface{} with backend-specific output.
	//     Common keys: "download_url", "form_url", "status", "tracking_id", "order_id"
	//   - error: nil on success; backend-specific error on failure
	Handle(ctx context.Context, payment *models.Payment, item *models.Item) (map[string]interface{}, error)

	// Validate checks configuration validity before item creation/update.
	//
	// Returns:
	//   - error: nil if config is valid; descriptive error otherwise
	Validate(config models.JSONMap) error

	// Metadata returns handler information for admin UI and discovery.
	//
	// Returns:
	//   - HandlerMetadata with name, description, required fields
	Metadata() HandlerMetadata
}

// HandlerMetadata describes a FulfillmentHandler for discovery and configuration.
type HandlerMetadata struct {
	// Type is the unique identifier for this handler (e.g., "digital_media").
	Type string `json:"type"`
	// DisplayName is a human-readable name (e.g., "Digital Media Download").
	DisplayName string `json:"display_name"`
	// Description explains the handler's purpose and use case.
	Description string `json:"description"`
	// RequiredFields lists configuration fields required for this handler.
	RequiredFields []Field `json:"required_fields"`
	// OptionalFields lists optional configuration fields.
	OptionalFields []Field `json:"optional_fields"`
}

// Field describes a configuration field for a handler.
type Field struct {
	// Name is the field key in the config JSON.
	Name string `json:"name"`
	// Type indicates the data type ("string", "number", "boolean", "secret", "object").
	Type string `json:"type"`
	// Description explains the field's purpose.
	Description string `json:"description"`
	// Example shows a sample value.
	Example string `json:"example"`
	// Validation describes any validation rule (regex, length limits, etc.).
	Validation string `json:"validation,omitempty"`
	// Required indicates if this field must be present.
	Required bool `json:"required"`
}

// HandlerRegistry defines the interface for managing fulfillment handlers.
type HandlerRegistry interface {
	// Register adds a handler to the registry.
	Register(handler FulfillmentHandler) error

	// Get retrieves a handler by its type.
	Get(handlerType string) (FulfillmentHandler, error)

	// All returns metadata for all registered handlers.
	All() map[string]HandlerMetadata
}

// Registry manages registered FulfillmentHandlers.
type Registry struct {
	handlers map[string]FulfillmentHandler
	mu       sync.RWMutex
}

// Verify that Registry implements HandlerRegistry at compile time.
var _ HandlerRegistry = (*Registry)(nil)

// NewRegistry creates a new handler registry.
func NewRegistry() HandlerRegistry {
	return &Registry{
		handlers: make(map[string]FulfillmentHandler),
	}
}

// Register adds a handler to the registry.
// Returns an error if a handler with the same type is already registered.
func (r *Registry) Register(handler FulfillmentHandler) error {
	if handler == nil {
		return fmt.Errorf("handler cannot be nil")
	}

	meta := handler.Metadata()
	if meta.Type == "" {
		return fmt.Errorf("handler type cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.handlers[meta.Type]; exists {
		return fmt.Errorf("handler type %q already registered", meta.Type)
	}

	r.handlers[meta.Type] = handler
	return nil
}

// Get retrieves a handler by its type.
// Returns an error if the handler type is not registered.
func (r *Registry) Get(handlerType string) (FulfillmentHandler, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	handler, exists := r.handlers[handlerType]
	if !exists {
		return nil, fmt.Errorf("handler type %q not found", handlerType)
	}

	return handler, nil
}

// All returns metadata for all registered handlers.
func (r *Registry) All() map[string]HandlerMetadata {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]HandlerMetadata, len(r.handlers))
	for _, handler := range r.handlers {
		meta := handler.Metadata()
		result[meta.Type] = meta
	}

	return result
}

// Config represents handler configuration settings.
type Config struct {
	Settings map[string]interface{} `json:"settings"`
}

// GetString retrieves a string value from config settings.
func (c Config) GetString(key string) string {
	if v, ok := c.Settings[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// GetInt retrieves an int value from config settings.
func (c Config) GetInt(key string) int {
	if v, ok := c.Settings[key]; ok {
		switch val := v.(type) {
		case float64:
			return int(val)
		case int:
			return val
		}
	}
	return 0
}

// GetBool retrieves a bool value from config settings.
func (c Config) GetBool(key string) bool {
	if v, ok := c.Settings[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

// Common error variables for handlers

var (
	// ErrPaymentNotConfirmed is returned when a handler receives an unconfirmed payment.
	ErrPaymentNotConfirmed = fmt.Errorf("payment not confirmed")
	// ErrInvalidConfig is returned when handler configuration is invalid.
	ErrInvalidConfig = fmt.Errorf("invalid handler configuration")
	// ErrFulfillmentFailed is returned when fulfillment processing fails.
	ErrFulfillmentFailed = fmt.Errorf("fulfillment failed")
)
