package pod

import (
	"context"
	"strconv"

	"github.com/opd-ai/store/pkg/printful"
)

// PrintfulProvider implements the Provider interface for Printful.
type PrintfulProvider struct {
	client *printful.Client
}

// NewPrintfulProvider creates a new Printful provider.
func NewPrintfulProvider(apiKey string) *PrintfulProvider {
	return &PrintfulProvider{
		client: printful.NewClient(apiKey),
	}
}

// CreateOrder creates a new order with Printful.
func (p *PrintfulProvider) CreateOrder(ctx context.Context, request *OrderRequest) (*OrderResponse, error) {
	// Convert generic request to Printful-specific format
	variantID, err := strconv.Atoi(request.VariantID)
	if err != nil {
		variantID = 0
	}

	orderReq := &printful.OrderRequest{
		Recipient: printful.Recipient{
			Name:        request.RecipientName,
			Address1:    request.RecipientAddress,
			City:        request.RecipientCity,
			StateCode:   request.RecipientState,
			CountryCode: request.RecipientCountry,
			Zip:         request.RecipientZip,
			Email:       request.RecipientEmail,
			Phone:       request.RecipientPhone,
		},
		Items: []printful.OrderItem{
			{
				VariantID: variantID,
				Quantity:  request.Quantity,
			},
		},
		ConfirmDraft: true,
	}

	// Add design file if provided
	if request.DesignURL != "" {
		orderReq.Items[0].Files = []printful.File{
			{
				Type: "default",
				URL:  request.DesignURL,
			},
		}
	}

	order, err := p.client.CreateOrder(ctx, orderReq)
	if err != nil {
		return nil, err
	}

	return &OrderResponse{
		OrderID:      order.OrderID,
		ExternalID:   order.ExternalID,
		Status:       order.Status,
		TrackingURL:  order.TrackingURL,
		ShippingDate: order.ShippingDate,
		CreatedAt:    order.Created.String(),
	}, nil
}

// GetStatus retrieves the current status of an order.
func (p *PrintfulProvider) GetStatus(ctx context.Context, orderID string) (*OrderStatusResponse, error) {
	status, err := p.client.GetOrderStatus(ctx, orderID)
	if err != nil {
		return nil, err
	}

	return &OrderStatusResponse{
		OrderID:      status.OrderID,
		Status:       status.Status,
		TrackingURL:  status.TrackingURL,
		ShippingDate: status.ShippingDate,
	}, nil
}

// CancelOrder cancels an existing order.
func (p *PrintfulProvider) CancelOrder(ctx context.Context, orderID string) error {
	return p.client.CancelOrder(ctx, orderID)
}

// Name returns the provider name.
func (p *PrintfulProvider) Name() string {
	return "printful"
}
