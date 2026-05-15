package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/opd-ai/store/pkg/handler"
	"github.com/opd-ai/store/pkg/metrics"
	"github.com/opd-ai/store/pkg/models"
)

// ShippingFormHandler collects shipping address after payment confirmation.
// It serves an address form to the customer and stores submissions for manual fulfillment.
type ShippingFormHandler struct{}

// NewShippingFormHandler creates a new shipping form handler.
func NewShippingFormHandler() *ShippingFormHandler {
	return &ShippingFormHandler{}
}

// Handle implements FulfillmentHandler.
func (h *ShippingFormHandler) Handle(ctx context.Context, payment *models.Payment, item *models.Item) (map[string]interface{}, error) {
	// Verify payment is confirmed
	if !payment.IsConfirmed() {
		metrics.HandlerErrors.WithLabelValues("shipping_form").Inc()
		return nil, handler.ErrPaymentNotConfirmed
	}

	// Check if this is an escrow payment
	if payment.EscrowEnabled {
		return h.handleEscrowPayment(ctx, payment, item)
	}

	// Fallback to single-sig flow (legacy)
	return h.handleSingleSigPayment(ctx, payment, item)
}

// handleSingleSigPayment handles traditional single-signature payments.
func (h *ShippingFormHandler) handleSingleSigPayment(ctx context.Context, payment *models.Payment, item *models.Item) (map[string]interface{}, error) {
	// Generate a form URL for the customer to submit their address
	formURL := fmt.Sprintf("https://store.example.com/fulfill/address/%s", payment.ID)
	timeout := 60 // minutes

	return map[string]interface{}{
		"form_url":        formURL,
		"status":          "awaiting_address",
		"timeout_minutes": timeout,
		"payment_id":      payment.ID,
	}, nil
}

// handleEscrowPayment handles multisig escrow payment workflows.
func (h *ShippingFormHandler) handleEscrowPayment(ctx context.Context, payment *models.Payment, item *models.Item) (map[string]interface{}, error) {
	// Escrow states: created → funded → address_submitted → shipped → released
	switch payment.EscrowState {
	case "created":
		return h.escrowStateCreated(payment)
	case "funded":
		return h.escrowStateFunded(payment)
	case "address_submitted":
		return h.escrowStateAddressSubmitted(payment)
	case "shipped":
		return h.escrowStateShipped(payment)
	case "released":
		return h.escrowStateReleased(payment)
	case "refunded":
		return h.escrowStateRefunded(payment)
	case "disputed":
		return h.escrowStateDisputed(payment)
	default:
		return nil, fmt.Errorf("unknown escrow state: %s", payment.EscrowState)
	}
}

// escrowStateCreated handles the initial escrow creation state.
func (h *ShippingFormHandler) escrowStateCreated(payment *models.Payment) (map[string]interface{}, error) {
	return map[string]interface{}{
		"status":       "awaiting_funding",
		"escrow_state": "created",
		"message":      "Waiting for payment to be sent to escrow address",
		"payment_id":   payment.ID,
	}, nil
}

// escrowStateFunded handles the state after escrow is funded.
func (h *ShippingFormHandler) escrowStateFunded(payment *models.Payment) (map[string]interface{}, error) {
	formURL := fmt.Sprintf("https://store.example.com/fulfill/address/%s", payment.ID)
	timeoutStr := ""
	if payment.EscrowTimeout != nil {
		timeoutStr = payment.EscrowTimeout.Format(time.RFC3339)
	}

	return map[string]interface{}{
		"status":       "awaiting_address",
		"form_url":     formURL,
		"escrow_state": "funded",
		"timeout":      timeoutStr,
		"message":      "Funds held in escrow. Please submit shipping address.",
	}, nil
}

// escrowStateAddressSubmitted handles the state after address is submitted.
func (h *ShippingFormHandler) escrowStateAddressSubmitted(payment *models.Payment) (map[string]interface{}, error) {
	result := map[string]interface{}{
		"status":       "processing",
		"escrow_state": "address_submitted",
		"message":      "Processing order. Funds held in escrow until shipment.",
	}

	// Include shipping info if available
	if payment.ShippingInfo != nil {
		result["shipping_address"] = payment.ShippingInfo
	}

	return result, nil
}

// escrowStateShipped handles the state after item is shipped.
func (h *ShippingFormHandler) escrowStateShipped(payment *models.Payment) (map[string]interface{}, error) {
	result := map[string]interface{}{
		"status":       "shipped",
		"escrow_state": "shipped",
		"message":      "Item shipped. Buyer must release funds or file dispute within 7 days.",
	}

	// Include tracking info if available
	if payment.FulfillmentResult != nil {
		if trackingURL, ok := payment.FulfillmentResult["tracking_url"].(string); ok {
			result["tracking_url"] = trackingURL
		}
		if trackingNumber, ok := payment.FulfillmentResult["tracking_number"].(string); ok {
			result["tracking_number"] = trackingNumber
		}
	}

	return result, nil
}

// escrowStateReleased handles the state after funds are released.
func (h *ShippingFormHandler) escrowStateReleased(payment *models.Payment) (map[string]interface{}, error) {
	return map[string]interface{}{
		"status":       "completed",
		"escrow_state": "released",
		"message":      "Order completed. Funds released to seller.",
	}, nil
}

// escrowStateRefunded handles the state after funds are refunded.
func (h *ShippingFormHandler) escrowStateRefunded(payment *models.Payment) (map[string]interface{}, error) {
	return map[string]interface{}{
		"status":       "refunded",
		"escrow_state": "refunded",
		"message":      "Order cancelled. Funds refunded to buyer.",
	}, nil
}

// escrowStateDisputed handles the disputed state.
func (h *ShippingFormHandler) escrowStateDisputed(payment *models.Payment) (map[string]interface{}, error) {
	result := map[string]interface{}{
		"status":       "disputed",
		"escrow_state": "disputed",
		"message":      "Order under dispute resolution with arbiter.",
	}

	if payment.DisputeReason != nil {
		result["dispute_reason"] = *payment.DisputeReason
	}
	if payment.DisputeResolution != nil {
		result["dispute_resolution"] = *payment.DisputeResolution
	}

	return result, nil
}

// Validate implements FulfillmentHandler.
func (h *ShippingFormHandler) Validate(config models.JSONMap) error {
	// Extract form fields configuration
	fieldsKey := "form_fields"
	if _, ok := config[fieldsKey]; !ok {
		return fmt.Errorf("missing required field: %s", fieldsKey)
	}

	// In a real implementation, validate field definitions
	// Ensure required fields like address1, city, state, country are defined

	return nil
}

// Metadata implements FulfillmentHandler.
func (h *ShippingFormHandler) Metadata() handler.HandlerMetadata {
	return handler.HandlerMetadata{
		Type:        "shipping_form",
		DisplayName: "Shipping Address Form",
		Description: "Collect shipping address from customer after payment. Manual fulfillment required.",
		RequiredFields: []handler.Field{
			{
				Name:        "form_fields",
				Type:        "object",
				Description: "Definition of address form fields to collect",
				Example:     `{"address1": {"label": "Street Address", "required": true}, "city": {...}}`,
				Required:    true,
			},
		},
		OptionalFields: []handler.Field{
			{
				Name:        "require_phone",
				Type:        "boolean",
				Description: "Whether phone number is required",
				Example:     "true",
				Required:    false,
			},
			{
				Name:        "require_notes",
				Type:        "boolean",
				Description: "Whether delivery notes field is shown",
				Example:     "false",
				Required:    false,
			},
			{
				Name:        "form_timeout_minutes",
				Type:        "number",
				Description: "Time limit for customer to submit address",
				Example:     "60",
				Required:    false,
			},
		},
	}
}

// FormData represents submitted shipping form data.
type FormData struct {
	Address1    string    `json:"address1"`
	Address2    string    `json:"address2"`
	City        string    `json:"city"`
	State       string    `json:"state"`
	PostalCode  string    `json:"postal_code"`
	Country     string    `json:"country"`
	Phone       string    `json:"phone"`
	Notes       string    `json:"notes"`
	SubmittedAt time.Time `json:"submitted_at"`
}

// ValidateFormData validates shipping form submission.
func ValidateFormData(data FormData) error {
	if data.Address1 == "" {
		return fmt.Errorf("address1 is required")
	}
	if data.City == "" {
		return fmt.Errorf("city is required")
	}
	if data.State == "" {
		return fmt.Errorf("state is required")
	}
	if data.PostalCode == "" {
		return fmt.Errorf("postal_code is required")
	}
	if data.Country == "" {
		return fmt.Errorf("country is required")
	}
	return nil
}
