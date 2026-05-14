package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"

	"github.com/opd-ai/store/pkg/models"
	"github.com/opd-ai/store/pkg/paywall"
)

// CreateCheckout initiates a payment checkout.
func (h *Handler) CreateCheckout(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ItemID string `json:"item_id"`
		Email  string `json:"email"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, http.StatusBadRequest, "Invalid request format")
		return
	}

	item, err := h.store.GetItem(r.Context(), req.ItemID)
	if err != nil {
		sendError(w, http.StatusNotFound, "Item not found")
		return
	}

	payment, err := h.store.CreatePayment(r.Context(), req.ItemID, item.Price, item.Currency)
	if err != nil {
		sendError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.savePayerEmail(r.Context(), payment, req.Email)

	invoice, err := h.createPaywallInvoice(r.Context(), payment)
	if err != nil {
		sendError(w, http.StatusInternalServerError, "Failed to create payment invoice")
		return
	}

	if err := h.store.UpdatePaymentInvoice(r.Context(), payment.ID, invoice.InvoiceID); err != nil {
		log.Printf("Failed to update payment invoice: %v", err)
		sendError(w, http.StatusInternalServerError, "Failed to update payment")
		return
	}

	response := h.buildCheckoutResponse(payment, invoice)
	sendJSON(w, http.StatusCreated, response)
}

// savePayerEmail stores the payer's email in the payment record.
func (h *Handler) savePayerEmail(ctx context.Context, payment *models.Payment, email string) {
	if payment.PayerInfo == nil {
		payment.PayerInfo = models.JSONMap{}
	}
	payment.PayerInfo["email"] = email

	if err := h.store.UpdatePaymentPayerInfo(ctx, payment.ID, payment.PayerInfo); err != nil {
		log.Printf("Failed to update payer info: %v", err)
	}
}

// createPaywallInvoice creates an invoice with the paywall service.
func (h *Handler) createPaywallInvoice(ctx context.Context, payment *models.Payment) (*paywall.Invoice, error) {
	callbackURL := fmt.Sprintf("%s/webhook/payment-confirmed", os.Getenv("STORE_PUBLIC_URL"))
	invoice, err := h.paywallClient.CreateInvoice(ctx, payment.Amount, payment.Currency, callbackURL)
	if err != nil {
		log.Printf("Failed to create invoice: %v", err)
		return nil, err
	}
	return invoice, nil
}

// buildCheckoutResponse constructs the checkout response from payment and invoice data.
func (h *Handler) buildCheckoutResponse(payment *models.Payment, invoice *paywall.Invoice) map[string]interface{} {
	return map[string]interface{}{
		"payment_id":      payment.ID,
		"invoice_id":      invoice.InvoiceID,
		"status":          payment.Status,
		"amount":          payment.Amount,
		"currency":        payment.Currency,
		"payment_address": invoice.PaymentAddress,
		"qr_code":         invoice.QRCode,
		"expires_at":      invoice.ExpiresAt,
	}
}

// GetPaymentStatus returns the status of a payment.
func (h *Handler) GetPaymentStatus(w http.ResponseWriter, r *http.Request) {
	payment := h.getPaymentOrError(w, r, "id")
	if payment == nil {
		return
	}

	// Poll remote status if payment is pending
	remoteStatus := h.pollPaywallStatus(r.Context(), payment)

	response := map[string]interface{}{
		"id":                 payment.ID,
		"invoice_id":         payment.InvoiceID,
		"item_id":            payment.ItemID,
		"status":             payment.Status,
		"amount":             payment.Amount,
		"currency":           payment.Currency,
		"confirmed_at":       payment.ConfirmedAt,
		"fulfilled_at":       payment.FulfilledAt,
		"fulfillment_result": payment.FulfillmentResult,
	}

	// Include remote status if available
	if remoteStatus != nil {
		response["remote_status"] = map[string]interface{}{
			"status":    remoteStatus.Status,
			"confirmed": remoteStatus.Confirmed,
		}
	}

	sendJSON(w, http.StatusOK, response)
}

// pollPaywallStatus checks the paywall for payment status and updates local state if needed.
func (h *Handler) pollPaywallStatus(ctx context.Context, payment *models.Payment) *paywall.InvoiceStatus {
	// Only poll if payment has invoice ID and is pending
	if payment.InvoiceID == "" || payment.Status != "pending" {
		return nil
	}

	// Get status from paywall
	status, err := h.paywallClient.GetInvoiceStatus(ctx, payment.InvoiceID)
	if err != nil {
		log.Printf("Failed to get invoice status from paywall: %v", err)
		return nil
	}

	// If confirmed, update local payment and auto-fulfill if enabled
	if status.Confirmed {
		h.handleConfirmedPayment(ctx, payment)
	}

	return status
}

// handleConfirmedPayment confirms payment and optionally auto-fulfills.
func (h *Handler) handleConfirmedPayment(ctx context.Context, payment *models.Payment) {
	// Confirm payment
	if err := h.store.ConfirmPayment(ctx, payment.ID, payment.InvoiceID); err != nil {
		log.Printf("Failed to update payment status: %v", err)
		return
	}

	// Reload payment to get updated status
	h.reloadPayment(ctx, payment)

	// Auto-fulfill if enabled
	if h.shouldAutoFulfill() {
		h.attemptAutoFulfill(ctx, payment)
	}
}

// reloadPayment updates the payment pointer with the latest data.
func (h *Handler) reloadPayment(ctx context.Context, payment *models.Payment) {
	updatedPayment, _ := h.store.GetPayment(ctx, payment.ID)
	if updatedPayment != nil {
		*payment = *updatedPayment
	}
}

// attemptAutoFulfill tries to fulfill payment and reloads on success.
func (h *Handler) attemptAutoFulfill(ctx context.Context, payment *models.Payment) {
	if err := h.store.FulfillPayment(ctx, payment.ID); err != nil {
		log.Printf("Failed to auto-fulfill payment %s: %v", payment.ID, err)
		return
	}

	// Reload payment to get fulfillment result
	h.reloadPayment(ctx, payment)
}

// shouldAutoFulfill checks if auto-fulfillment is enabled.
func (h *Handler) shouldAutoFulfill() bool {
	autoFulfill := os.Getenv("STORE_AUTO_FULFILL")
	return autoFulfill == "" || autoFulfill == "true"
}

// SubmitPaymentForm submits form data for a payment.
func (h *Handler) SubmitPaymentForm(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	paymentID := vars["id"]

	// Parse form data
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendError(w, http.StatusBadRequest, "Failed to read request body")
		return
	}

	var formData map[string]interface{}
	if err := json.Unmarshal(body, &formData); err != nil {
		sendError(w, http.StatusBadRequest, "Invalid request format")
		return
	}

	// Store form submission
	submission, err := h.store.SubmitFormData(r.Context(), paymentID, formData)
	if err != nil {
		sendError(w, http.StatusInternalServerError, err.Error())
		return
	}

	sendJSON(w, http.StatusCreated, map[string]interface{}{
		"id":         submission.ID,
		"payment_id": submission.PaymentID,
		"status":     "submitted",
	})
}

// TrackDownload records a download attempt and checks rate limits.
func (h *Handler) TrackDownload(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	paymentID := vars["id"]

	// Get and validate payment and item
	payment, item, err := h.getPaymentWithItem(r.Context(), paymentID)
	if err != nil {
		sendError(w, http.StatusNotFound, err.Error())
		return
	}

	// Validate download eligibility
	if err := h.validateDownloadEligibility(payment); err != nil {
		sendError(w, err.(*downloadError).statusCode, err.Error())
		return
	}

	// Check download limit
	if err := h.checkDownloadLimits(r.Context(), paymentID, item.BackendConfig); err != nil {
		sendError(w, err.(*downloadError).statusCode, err.Error())
		return
	}

	// Record the download
	if err := h.store.RecordDownload(r.Context(), paymentID, r.RemoteAddr, r.UserAgent()); err != nil {
		sendError(w, http.StatusInternalServerError, err.Error())
		return
	}

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "tracked",
		"payment": payment,
		"item":    item,
	})
}

// downloadError carries HTTP status codes for download errors.
type downloadError struct {
	statusCode int
	message    string
}

func (e *downloadError) Error() string {
	return e.message
}

// getPaymentWithItem retrieves and validates payment and item.
func (h *Handler) getPaymentWithItem(ctx context.Context, paymentID string) (*models.Payment, *models.Item, error) {
	payment, err := h.store.GetPayment(ctx, paymentID)
	if err != nil {
		return nil, nil, fmt.Errorf("Payment not found")
	}

	item, err := h.store.GetItem(ctx, payment.ItemID)
	if err != nil {
		return nil, nil, fmt.Errorf("Item not found")
	}

	return payment, item, nil
}

// validateDownloadEligibility checks payment status and expiration.
func (h *Handler) validateDownloadEligibility(payment *models.Payment) error {
	if payment.Status != "fulfilled" {
		return &downloadError{http.StatusForbidden, "Payment not fulfilled"}
	}

	// Check expiration from fulfillment_result
	if expiresAtStr, ok := payment.FulfillmentResult["expires_at"].(string); ok {
		expiresAt, err := time.Parse(time.RFC3339, expiresAtStr)
		if err == nil && time.Now().After(expiresAt) {
			return &downloadError{http.StatusGone, "Download link has expired"}
		}
	}

	return nil
}

// checkDownloadLimits validates download count against configured limits.
func (h *Handler) checkDownloadLimits(ctx context.Context, paymentID string, config models.JSONMap) error {
	maxDownloads := 0
	if val, ok := config["max_downloads"].(float64); ok {
		maxDownloads = int(val)
	}

	limitExceeded, err := h.store.CheckDownloadLimit(ctx, paymentID, maxDownloads)
	if err != nil {
		return &downloadError{http.StatusInternalServerError, err.Error()}
	}

	if limitExceeded {
		return &downloadError{http.StatusTooManyRequests, "Download limit exceeded"}
	}

	return nil
}
