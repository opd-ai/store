package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	storesvc "github.com/opd-ai/store/pkg/store"
)

// WebhookPaymentConfirmed handles payment confirmation webhooks from the paywall service.
func (h *Handler) WebhookPaymentConfirmed(w http.ResponseWriter, r *http.Request) {
	// Read and verify webhook
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Failed to read webhook body: %v", err)
		sendError(w, http.StatusBadRequest, "Failed to read request body")
		return
	}
	defer func() { _ = r.Body.Close() }()

	// Verify signature
	if err := h.verifyWebhookSignature(r, body); err != nil {
		sendError(w, http.StatusUnauthorized, err.Error())
		return
	}

	// Parse payload
	payload, err := parseWebhookPayload(body)
	if err != nil {
		log.Printf("Failed to parse webhook payload: %v", err)
		sendError(w, http.StatusBadRequest, "Invalid payload format")
		return
	}

	// Process payment confirmation
	if err := h.processPaymentConfirmation(r.Context(), payload); err != nil {
		log.Printf("Failed to process payment confirmation: %v", err)
		sendError(w, http.StatusInternalServerError, err.Error())
		return
	}

	sendJSON(w, http.StatusOK, map[string]string{
		"status":     "confirmed",
		"payment_id": payload.PaymentID,
	})
}

// webhookPayload represents the webhook payload structure.
type webhookPayload struct {
	InvoiceID   string
	Status      string
	PaymentHash string
	Amount      string
	Currency    string
	PaymentID   string
}

// verifyWebhookSignature verifies the webhook signature if configured.
func (h *Handler) verifyWebhookSignature(r *http.Request, body []byte) error {
	signature := r.Header.Get("X-Webhook-Signature")
	webhookSecret := os.Getenv("STORE_PAYWALL_WEBHOOK_SECRET")

	if webhookSecret == "" || signature == "" {
		return nil // No verification configured
	}

	valid, err := h.paywallClient.VerifyWebhook(signature, body, webhookSecret)
	if err != nil {
		log.Printf("Failed to verify webhook signature: %v", err)
		return fmt.Errorf("failed to verify signature")
	}

	if !valid {
		log.Printf("Invalid webhook signature")
		return fmt.Errorf("invalid signature")
	}

	return nil
}

// parseWebhookPayload parses the webhook payload from JSON.
func parseWebhookPayload(body []byte) (*webhookPayload, error) {
	var raw struct {
		InvoiceID   string `json:"invoice_id"`
		Status      string `json:"status"`
		PaymentHash string `json:"tx_hash"`
		Amount      string `json:"amount"`
		Currency    string `json:"currency"`
	}

	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}

	return &webhookPayload{
		InvoiceID:   raw.InvoiceID,
		Status:      raw.Status,
		PaymentHash: raw.PaymentHash,
		Amount:      raw.Amount,
		Currency:    raw.Currency,
	}, nil
}

// processPaymentConfirmation confirms and optionally fulfills a payment.
func (h *Handler) processPaymentConfirmation(ctx context.Context, payload *webhookPayload) error {
	// Get payment by invoice ID
	payment, err := h.store.GetPaymentByInvoiceID(ctx, payload.InvoiceID)
	if err != nil {
		return fmt.Errorf("payment not found for invoice %s: %w", payload.InvoiceID, err)
	}
	payload.PaymentID = payment.ID

	// Confirm payment with hash
	if err := h.store.ConfirmPayment(ctx, payment.ID, payload.PaymentHash); err != nil {
		return fmt.Errorf("failed to confirm payment %s: %w", payment.ID, err)
	}

	log.Printf("Payment %s confirmed via webhook (invoice: %s, hash: %s)", payment.ID, payload.InvoiceID, payload.PaymentHash)

	// Auto-fulfill if enabled
	if storesvc.ShouldAutoFulfill() {
		if err := h.store.FulfillPayment(ctx, payment.ID); err != nil {
			log.Printf("Failed to auto-fulfill payment %s: %v", payment.ID, err)
			// Don't return error - payment is confirmed, fulfillment can be retried
		} else {
			log.Printf("Payment %s auto-fulfilled", payment.ID)
		}
	}

	return nil
}
