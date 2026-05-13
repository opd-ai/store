package pod

import (
	"context"
	"fmt"
	"time"
)

// RedbubbleProvider implements the Provider interface for Redbubble.
// This is a stub implementation. Real integration requires Redbubble API credentials.
type RedbubbleProvider struct {
	apiKey string
}

// NewRedbubbleProvider creates a new Redbubble provider.
func NewRedbubbleProvider(apiKey string) *RedbubbleProvider {
	return &RedbubbleProvider{
		apiKey: apiKey,
	}
}

// CreateOrder creates a new order with Redbubble (stub implementation).
func (r *RedbubbleProvider) CreateOrder(ctx context.Context, request *OrderRequest) (*OrderResponse, error) {
	// NOTE: Redbubble does not provide a public API for order creation.
	// This is a stub implementation that returns mock data for testing.
	// For production use with Redbubble, consider alternative integration methods.
	orderID := fmt.Sprintf("RB-%d", time.Now().Unix())

	return &OrderResponse{
		OrderID:      orderID,
		ExternalID:   orderID,
		Status:       "processing",
		TrackingURL:  fmt.Sprintf("https://redbubble.com/track/%s", orderID),
		ShippingDate: time.Now().Add(5 * 24 * time.Hour).Format("2006-01-02"),
		CreatedAt:    time.Now().Format(time.RFC3339),
	}, nil
}

// GetStatus retrieves the current status of an order (stub implementation).
func (r *RedbubbleProvider) GetStatus(ctx context.Context, orderID string) (*OrderStatusResponse, error) {
	// NOTE: Redbubble does not provide a public API for order status.
	// This is a stub implementation that returns mock data for testing.
	return &OrderStatusResponse{
		OrderID:      orderID,
		Status:       "fulfilled",
		TrackingURL:  fmt.Sprintf("https://redbubble.com/track/%s", orderID),
		ShippingDate: time.Now().Format("2006-01-02"),
		LastUpdated:  time.Now().Format(time.RFC3339),
	}, nil
}

// CancelOrder cancels an existing order (stub implementation).
func (r *RedbubbleProvider) CancelOrder(ctx context.Context, orderID string) error {
	// NOTE: Redbubble does not provide a public API for order cancellation.
	// This is a stub implementation for interface compliance.
	return nil
}

// Name returns the provider name.
func (r *RedbubbleProvider) Name() string {
	return "redbubble"
}
