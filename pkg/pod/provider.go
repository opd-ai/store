// Package pod provides print-on-demand provider integrations.
// It defines a Provider interface supporting Printful and other fulfillment services.
//
// Supported providers: Printful (more coming soon).
//
// Example usage:
//
//	provider, err := pod.NewProvider("printful", apiKey)
//	order, err := provider.CreateOrder(ctx, orderRequest)
//	status, err := provider.GetStatus(ctx, orderID)
package pod

import (
	"context"
	"fmt"
)

// Provider defines the interface for print-on-demand service providers.
type Provider interface {
	// CreateOrder creates a new order with the provider.
	CreateOrder(ctx context.Context, request *OrderRequest) (*OrderResponse, error)

	// GetStatus retrieves the current status of an order.
	GetStatus(ctx context.Context, orderID string) (*OrderStatusResponse, error)

	// CancelOrder cancels an existing order.
	CancelOrder(ctx context.Context, orderID string) error

	// Name returns the provider name.
	Name() string
}

// OrderRequest represents a generic order creation request.
type OrderRequest struct {
	// Recipient information
	RecipientName    string
	RecipientAddress string
	RecipientCity    string
	RecipientState   string
	RecipientZip     string
	RecipientCountry string
	RecipientEmail   string
	RecipientPhone   string

	// Product information
	ProductID string
	VariantID string
	Quantity  int
	DesignURL string // URL to design file for printing

	// Additional metadata
	Metadata map[string]interface{}
}

// OrderResponse represents the response from order creation.
type OrderResponse struct {
	OrderID      string
	ExternalID   string
	Status       string
	TrackingURL  string
	ShippingDate string
	CreatedAt    string
}

// OrderStatusResponse represents the current status of an order.
type OrderStatusResponse struct {
	OrderID      string
	Status       string
	TrackingURL  string
	ShippingDate string
	LastUpdated  string
}

// NewProvider creates a provider instance based on the provider name.
func NewProvider(providerName, apiKey string) (Provider, error) {
	switch providerName {
	case "printful":
		return NewPrintfulProvider(apiKey), nil
	default:
		return nil, fmt.Errorf("unsupported provider: %s", providerName)
	}
}
