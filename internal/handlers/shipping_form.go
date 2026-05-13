package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/opd-ai/store/pkg/handler"
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
		return nil, handler.ErrPaymentNotConfirmed
	}

	// Generate a form URL for the customer to submit their address
	// In a real implementation, this would persist the form state and return a unique URL
	formURL := fmt.Sprintf("https://store.example.com/fulfill/address/%s", payment.ID)
	timeout := 60 // minutes

	return map[string]interface{}{
		"form_url":        formURL,
		"status":          "awaiting_address",
		"timeout_minutes": timeout,
		"payment_id":      payment.ID,
	}, nil
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
