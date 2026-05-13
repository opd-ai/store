package handlers

import (
	"context"
	"fmt"

	"github.com/opd-ai/store/pkg/handler"
	"github.com/opd-ai/store/pkg/models"
)

// PrintOnDemandHandler delegates to external print-on-demand services.
// It supports providers like Printful, Redbubble, and similar PoD platforms.
type PrintOnDemandHandler struct{}

// NewPrintOnDemandHandler creates a new print-on-demand handler.
func NewPrintOnDemandHandler() *PrintOnDemandHandler {
	return &PrintOnDemandHandler{}
}

// Handle implements FulfillmentHandler.
func (h *PrintOnDemandHandler) Handle(ctx context.Context, payment *models.Payment, item *models.Item) (map[string]interface{}, error) {
	// Verify payment is confirmed
	if !payment.IsConfirmed() {
		return nil, handler.ErrPaymentNotConfirmed
	}

	// Extract PoD configuration from item backend config
	backendConfig := item.BackendConfig
	if backendConfig == nil {
		return nil, fmt.Errorf("missing backend configuration")
	}

	provider, ok := backendConfig["provider"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid provider in configuration")
	}

	// In a real implementation:
	// 1. Extract API endpoint and credentials from config
	// 2. Build order payload with product ID, variant, customer address
	// 3. Call respective provider API (Printful, Redbubble, etc.)
	// 4. Persist order ID and tracking URL
	// 5. Set up webhook listener for fulfillment status updates

	// For this example, simulate a Printful API call
	orderID := fmt.Sprintf("POD-%s", payment.ID[:8])
	trackingURL := fmt.Sprintf("https://%s.example.com/track/%s", provider, orderID)

	return map[string]interface{}{
		"provider":            provider,
		"order_id":            orderID,
		"tracking_url":        trackingURL,
		"status":              "processing",
		"estimated_ship_date": "2026-05-20",
	}, nil
}

// Validate implements FulfillmentHandler.
func (h *PrintOnDemandHandler) Validate(config models.JSONMap) error {
	// Check required fields
	requiredFields := []string{"provider", "api_key", "product_mapping"}
	for _, field := range requiredFields {
		if _, ok := config[field]; !ok {
			return fmt.Errorf("missing required field: %s", field)
		}
	}

	// Validate provider
	provider, ok := config["provider"].(string)
	if !ok {
		return fmt.Errorf("invalid provider type")
	}

	validProviders := map[string]bool{
		"printful":  true,
		"redbubble": true,
		"teespring": true,
	}

	if !validProviders[provider] {
		return fmt.Errorf("unsupported provider: %s", provider)
	}

	return nil
}

// Metadata implements FulfillmentHandler.
func (h *PrintOnDemandHandler) Metadata() handler.HandlerMetadata {
	return handler.HandlerMetadata{
		Type:        "pod",
		DisplayName: "Print-on-Demand Integration",
		Description: "Automatically create orders with print-on-demand providers (Printful, Redbubble, etc). Integrates with PoD vendor APIs for seamless fulfillment.",
		RequiredFields: []handler.Field{
			{
				Name:        "provider",
				Type:        "string",
				Description: "Print-on-demand service provider",
				Example:     "printful",
				Validation:  "must be 'printful', 'redbubble', or 'teespring'",
				Required:    true,
			},
			{
				Name:        "api_key",
				Type:        "secret",
				Description: "API key for authentication with PoD provider",
				Example:     "sk_live_...",
				Required:    true,
			},
			{
				Name:        "product_mapping",
				Type:        "object",
				Description: "Map of item IDs to PoD product/variant IDs",
				Example:     `{"item-123": {"product_id": 456, "variant_id": 789}}`,
				Required:    true,
			},
		},
		OptionalFields: []handler.Field{
			{
				Name:        "api_url",
				Type:        "string",
				Description: "Base URL for API endpoint (if not using provider default)",
				Example:     "https://api.printful.com",
				Required:    false,
			},
			{
				Name:        "webhook_secret",
				Type:        "secret",
				Description: "Webhook secret for order status updates",
				Example:     "whsec_...",
				Required:    false,
			},
			{
				Name:        "default_size",
				Type:        "string",
				Description: "Default size if not specified per item",
				Example:     "L",
				Required:    false,
			},
		},
	}
}

// PodOrder represents an order created with a PoD provider.
type PodOrder struct {
	Provider          string                 `json:"provider"`
	OrderID           string                 `json:"order_id"`
	ItemID            string                 `json:"item_id"`
	PaymentID         string                 `json:"payment_id"`
	Status            string                 `json:"status"`
	TrackingURL       string                 `json:"tracking_url"`
	EstimatedShipDate string                 `json:"estimated_ship_date"`
	Metadata          map[string]interface{} `json:"metadata"`
}
