package pod

import (
	"context"
	"fmt"
	"time"
)

// TeespringProvider implements the Provider interface for Teespring.
// This is a stub implementation. Real integration requires Teespring API credentials.
type TeespringProvider struct {
	apiKey string
}

// NewTeespringProvider creates a new Teespring provider.
func NewTeespringProvider(apiKey string) *TeespringProvider {
	return &TeespringProvider{
		apiKey: apiKey,
	}
}

// CreateOrder creates a new order with Teespring (stub implementation).
func (t *TeespringProvider) CreateOrder(ctx context.Context, request *OrderRequest) (*OrderResponse, error) {
	// TODO: Implement actual Teespring API integration
	// For now, return a mock response
	orderID := fmt.Sprintf("TS-%d", time.Now().Unix())

	return &OrderResponse{
		OrderID:      orderID,
		ExternalID:   orderID,
		Status:       "processing",
		TrackingURL:  fmt.Sprintf("https://teespring.com/track/%s", orderID),
		ShippingDate: time.Now().Add(7 * 24 * time.Hour).Format("2006-01-02"),
		CreatedAt:    time.Now().Format(time.RFC3339),
	}, nil
}

// GetStatus retrieves the current status of an order (stub implementation).
func (t *TeespringProvider) GetStatus(ctx context.Context, orderID string) (*OrderStatusResponse, error) {
	// TODO: Implement actual Teespring status API
	return &OrderStatusResponse{
		OrderID:      orderID,
		Status:       "fulfilled",
		TrackingURL:  fmt.Sprintf("https://teespring.com/track/%s", orderID),
		ShippingDate: time.Now().Format("2006-01-02"),
		LastUpdated:  time.Now().Format(time.RFC3339),
	}, nil
}

// CancelOrder cancels an existing order (stub implementation).
func (t *TeespringProvider) CancelOrder(ctx context.Context, orderID string) error {
	// TODO: Implement actual Teespring cancel API
	return nil
}

// Name returns the provider name.
func (t *TeespringProvider) Name() string {
	return "teespring"
}
