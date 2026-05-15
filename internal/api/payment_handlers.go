package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gorilla/mux"

	"github.com/opd-ai/store/pkg/models"
	"github.com/opd-ai/store/pkg/paywall"
	storesvc "github.com/opd-ai/store/pkg/store"
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

	// Determine if escrow should be used based on item backend type and config
	useEscrow := h.determineEscrowMode(item.BackendType)

	// Log escrow decision for visibility (actual escrow activation would require additional integration)
	if useEscrow {
		log.Printf("Payment for item %s (%s backend) will use escrow mode based on config", item.ID, item.BackendType)
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

// determineEscrowMode determines if escrow should be used based on backend type and config.
func (h *Handler) determineEscrowMode(backendType string) bool {
	if h.config == nil {
		return false
	}

	switch backendType {
	case "digital_media":
		return h.config.PaymentModeDigital == "multisig-escrow"
	case "shipping_form":
		return h.config.PaymentModeShipping == "multisig-escrow"
	case "pod":
		return h.config.PaymentModePOD == "multisig-escrow"
	default:
		// Default to single-sig for custom handlers
		return false
	}
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
	remoteStatus := storesvc.PollPaymentStatus(r.Context(), h.paywallClient, payment)

	// If confirmed remotely, update local state
	if remoteStatus != nil && remoteStatus.Confirmed && payment.Status == "pending" {
		if err := storesvc.ConfirmAndFulfill(r.Context(), h.store, payment, storesvc.ShouldAutoFulfill()); err != nil {
			log.Printf("Failed to process confirmation: %v", err)
		}
		// Reload payment to get updated status
		payment, _ = h.store.GetPayment(r.Context(), payment.ID)
	}

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

// ServeDownload serves the actual file for download.
func (h *Handler) ServeDownload(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	paymentID := vars["payment_id"]

	// Get payment and item
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

	// Check download limits
	if err := h.checkDownloadLimits(r.Context(), paymentID, item.BackendConfig); err != nil {
		sendError(w, err.(*downloadError).statusCode, err.Error())
		return
	}

	// Only support digital_media backend for direct downloads
	if item.BackendType != "digital_media" {
		sendError(w, http.StatusBadRequest, "Item does not support direct download")
		return
	}

	// Get storage configuration
	config := item.BackendConfig
	storage := "local"
	if s, ok := config["storage"].(string); ok {
		storage = s
	}

	// For S3, redirect to pre-signed URL
	if storage == "s3" {
		downloadURL, ok := payment.FulfillmentResult["download_url"].(string)
		if !ok || downloadURL == "" {
			sendError(w, http.StatusInternalServerError, "Download URL not available")
			return
		}
		http.Redirect(w, r, downloadURL, http.StatusTemporaryRedirect)
		// Record download after redirect
		_ = h.store.RecordDownload(r.Context(), paymentID, r.RemoteAddr, r.UserAgent())
		return
	}

	// For local storage, serve the file
	filePath, ok := config["file_path"].(string)
	if !ok || filePath == "" {
		sendError(w, http.StatusInternalServerError, "File path not configured")
		return
	}

	// Resolve relative paths from upload directory
	uploadsDir := os.Getenv("STORE_UPLOADS_DIR")
	if uploadsDir == "" {
		uploadsDir = "./data/uploads"
	}

	// If filePath is not absolute, use uploads directory
	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(uploadsDir, filePath)
	}

	// Check if file exists
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			sendError(w, http.StatusNotFound, "File not found")
		} else {
			sendError(w, http.StatusInternalServerError, "Failed to access file")
		}
		return
	}

	// Open file
	file, err := os.Open(filePath)
	if err != nil {
		sendError(w, http.StatusInternalServerError, "Failed to open file")
		return
	}
	defer file.Close()

	// Record download before serving
	if err := h.store.RecordDownload(r.Context(), paymentID, r.RemoteAddr, r.UserAgent()); err != nil {
		log.Printf("Failed to record download: %v", err)
		// Continue with download even if recording fails
	}

	// Set headers for file download
	filename := filepath.Base(filePath)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", fileInfo.Size()))

	// Stream file to client
	if _, err := io.Copy(w, file); err != nil {
		log.Printf("Error streaming file: %v", err)
	}
}
