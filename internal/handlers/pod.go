package handlers

import (
	"context"
	"fmt"

	"github.com/opd-ai/store/pkg/handler"
	"github.com/opd-ai/store/pkg/models"
	"github.com/opd-ai/store/pkg/pod"
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

	// Extract provider configuration
	providerName, apiKey, err := extractProviderConfig(item.BackendConfig)
	if err != nil {
		return nil, err
	}

	// Create provider instance
	provider, err := pod.NewProvider(providerName, apiKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create provider: %w", err)
	}

	// Extract variant ID for this item
	variantID, err := extractVariantID(item.BackendConfig, item.ID)
	if err != nil {
		return nil, err
	}

	// Extract recipient information
	recipientInfo, err := extractRecipientFromPayerInfo(payment.PayerInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to extract recipient info: %w", err)
	}

	// Build order request
	orderReq := buildOrderRequest(recipientInfo, variantID, item.BackendConfig, item.ID)

	// Create order with provider
	orderResp, err := provider.CreateOrder(ctx, orderReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create order with %s: %w", provider.Name(), err)
	}

	return map[string]interface{}{
		"provider":      provider.Name(),
		"order_id":      orderResp.OrderID,
		"external_id":   orderResp.ExternalID,
		"status":        orderResp.Status,
		"tracking_url":  orderResp.TrackingURL,
		"shipping_date": orderResp.ShippingDate,
		"created_at":    orderResp.CreatedAt,
	}, nil
}

// extractProviderConfig extracts provider name and API key from backend config.
func extractProviderConfig(backendConfig models.JSONMap) (string, string, error) {
	if backendConfig == nil {
		return "", "", fmt.Errorf("missing backend configuration")
	}

	providerName, ok := backendConfig["provider"].(string)
	if !ok {
		return "", "", fmt.Errorf("missing or invalid provider in configuration")
	}

	apiKey, ok := backendConfig["api_key"].(string)
	if !ok || apiKey == "" {
		return "", "", fmt.Errorf("missing or invalid api_key in configuration")
	}

	return providerName, apiKey, nil
}

// extractVariantID extracts the variant ID for a specific item from the product mapping.
func extractVariantID(backendConfig models.JSONMap, itemID string) (string, error) {
	productMapping, ok := backendConfig["product_mapping"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("missing or invalid product_mapping in configuration")
	}

	itemMapping, ok := productMapping[itemID].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("no product mapping found for item %s", itemID)
	}

	variantID, _ := itemMapping["variant_id"].(string)
	if variantID == "" {
		// Try as float64 (JSON number)
		if variantFloat, ok := itemMapping["variant_id"].(float64); ok {
			variantID = fmt.Sprintf("%.0f", variantFloat)
		} else {
			return "", fmt.Errorf("missing or invalid variant_id in product mapping")
		}
	}

	return variantID, nil
}

// buildOrderRequest creates an order request from recipient info and configuration.
func buildOrderRequest(recipient *recipientInfo, variantID string, backendConfig models.JSONMap, itemID string) *pod.OrderRequest {
	orderReq := &pod.OrderRequest{
		RecipientName:    recipient.Name,
		RecipientAddress: recipient.Address,
		RecipientCity:    recipient.City,
		RecipientState:   recipient.State,
		RecipientZip:     recipient.Zip,
		RecipientCountry: recipient.Country,
		RecipientEmail:   recipient.Email,
		RecipientPhone:   recipient.Phone,
		VariantID:        variantID,
		Quantity:         1,
	}

	// Extract design URL if provided
	if productMapping, ok := backendConfig["product_mapping"].(map[string]interface{}); ok {
		if itemMapping, ok := productMapping[itemID].(map[string]interface{}); ok {
			if designURL, ok := itemMapping["design_url"].(string); ok {
				orderReq.DesignURL = designURL
			}
		}
	}

	return orderReq
}

// recipientInfo holds structured recipient data.
type recipientInfo struct {
	Name    string
	Address string
	City    string
	State   string
	Zip     string
	Country string
	Email   string
	Phone   string
}

// extractRecipientFromPayerInfo extracts shipping recipient information from payment payer info.
func extractRecipientFromPayerInfo(payerInfo models.JSONMap) (*recipientInfo, error) {
	if payerInfo == nil {
		return nil, fmt.Errorf("payer info is nil")
	}

	// Try to extract standard fields
	name, _ := payerInfo["name"].(string)
	address1, _ := payerInfo["address1"].(string)
	address2, _ := payerInfo["address2"].(string)
	city, _ := payerInfo["city"].(string)
	stateCode, _ := payerInfo["state_code"].(string)
	countryCode, _ := payerInfo["country_code"].(string)
	zip, _ := payerInfo["zip"].(string)
	email, _ := payerInfo["email"].(string)
	phone, _ := payerInfo["phone"].(string)

	// Validate required fields
	if name == "" || address1 == "" || city == "" || countryCode == "" || zip == "" {
		return nil, fmt.Errorf("missing required shipping information (name, address1, city, country_code, zip)")
	}

	// Combine address lines if address2 exists
	fullAddress := address1
	if address2 != "" {
		fullAddress = address1 + ", " + address2
	}

	return &recipientInfo{
		Name:    name,
		Address: fullAddress,
		City:    city,
		State:   stateCode,
		Zip:     zip,
		Country: countryCode,
		Email:   email,
		Phone:   phone,
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
