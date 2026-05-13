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
	"github.com/opd-ai/store/pkg/pod"
	"github.com/opd-ai/store/pkg/store"
)

// Handler encapsulates HTTP handlers for the store API.
type Handler struct {
	store         *store.Store
	paywallClient *paywall.Client
}

// NewHandler creates a new API handler.
func NewHandler(s *store.Store, paywallClient *paywall.Client) *Handler {
	return &Handler{
		store:         s,
		paywallClient: paywallClient,
	}
}

// JSON response wrapper
type JSONResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// HealthHandler responds with server health status.
func HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// CORSMiddleware adds CORS headers.
func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Admin-Token")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// LoggingMiddleware logs HTTP requests.
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s", r.Method, r.RequestURI, r.RemoteAddr)
		next.ServeHTTP(w, r)
	})
}

// requireAdminToken validates admin authentication token.
func requireAdminToken(r *http.Request) error {
	token := r.Header.Get("X-Admin-Token")
	expectedToken := os.Getenv("STORE_ADMIN_TOKEN")
	if token != expectedToken || expectedToken == "" {
		return fmt.Errorf("unauthorized")
	}
	return nil
}

// GetCatalog returns all categories, items, and tags.
func (h *Handler) GetCatalog(w http.ResponseWriter, r *http.Request) {
	catalog, err := h.store.GetCatalog(r.Context())
	if err != nil {
		sendError(w, http.StatusInternalServerError, err.Error())
		return
	}

	sendJSON(w, http.StatusOK, catalog)
}

// GetItem returns a single item by ID.
func (h *Handler) GetItem(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	itemID := vars["id"]

	item, err := h.store.GetItem(r.Context(), itemID)
	if err != nil {
		sendError(w, http.StatusNotFound, "Item not found")
		return
	}

	sendJSON(w, http.StatusOK, item)
}

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

	// Get item
	item, err := h.store.GetItem(r.Context(), req.ItemID)
	if err != nil {
		sendError(w, http.StatusNotFound, "Item not found")
		return
	}

	// Create payment
	payment, err := h.store.CreatePayment(r.Context(), req.ItemID, item.Price, item.Currency)
	if err != nil {
		sendError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Store payer email
	if payment.PayerInfo == nil {
		payment.PayerInfo = models.JSONMap{}
	}
	payment.PayerInfo["email"] = req.Email

	// Save payer info to database
	if err := h.store.UpdatePaymentPayerInfo(r.Context(), payment.ID, payment.PayerInfo); err != nil {
		log.Printf("Failed to update payer info: %v", err)
	}

	// Create invoice with paywall service
	callbackURL := fmt.Sprintf("%s/webhook/payment-confirmed", os.Getenv("STORE_PUBLIC_URL"))
	invoice, err := h.paywallClient.CreateInvoice(r.Context(), payment.Amount, payment.Currency, callbackURL)
	if err != nil {
		log.Printf("Failed to create invoice: %v", err)
		sendError(w, http.StatusInternalServerError, "Failed to create payment invoice")
		return
	}

	// Update payment with invoice ID
	if err := h.store.UpdatePaymentInvoice(r.Context(), payment.ID, invoice.InvoiceID); err != nil {
		log.Printf("Failed to update payment invoice: %v", err)
		sendError(w, http.StatusInternalServerError, "Failed to update payment")
		return
	}

	sendJSON(w, http.StatusCreated, map[string]interface{}{
		"payment_id":      payment.ID,
		"invoice_id":      invoice.InvoiceID,
		"status":          payment.Status,
		"amount":          payment.Amount,
		"currency":        payment.Currency,
		"payment_address": invoice.PaymentAddress,
		"qr_code":         invoice.QRCode,
		"expires_at":      invoice.ExpiresAt,
	})
}

// GetPaymentStatus returns the status of a payment.
func (h *Handler) GetPaymentStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	paymentID := vars["id"]

	payment, err := h.store.GetPayment(r.Context(), paymentID)
	if err != nil {
		sendError(w, http.StatusNotFound, "Payment not found")
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

// ListHandlers returns metadata for all registered handlers.
func (h *Handler) ListHandlers(w http.ResponseWriter, r *http.Request) {
	handlers := h.store.HandlerMetadata()
	sendJSON(w, http.StatusOK, handlers)
}

// ListPayments lists payments with optional filters.
func (h *Handler) ListPayments(w http.ResponseWriter, r *http.Request) {
	if err := requireAdminToken(r); err != nil {
		sendError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	filters := map[string]interface{}{
		"status":  r.URL.Query().Get("status"),
		"item_id": r.URL.Query().Get("item_id"),
	}

	payments, err := h.store.ListPayments(r.Context(), filters)
	if err != nil {
		sendError(w, http.StatusInternalServerError, err.Error())
		return
	}

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"payments": payments,
	})
}

// ConfirmPayment confirms a payment after paywall verification.
func (h *Handler) ConfirmPayment(w http.ResponseWriter, r *http.Request) {
	if err := requireAdminToken(r); err != nil {
		sendError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	vars := mux.Vars(r)
	paymentID := vars["id"]

	var req struct {
		PaymentHash string `json:"payment_hash"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, http.StatusBadRequest, "Invalid request format")
		return
	}

	if err := h.store.ConfirmPayment(r.Context(), paymentID, req.PaymentHash); err != nil {
		sendError(w, http.StatusInternalServerError, err.Error())
		return
	}

	sendJSON(w, http.StatusOK, map[string]string{"status": "confirmed"})
}

// FulfillPayment triggers fulfillment for a confirmed payment.
func (h *Handler) FulfillPayment(w http.ResponseWriter, r *http.Request) {
	if err := requireAdminToken(r); err != nil {
		sendError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	vars := mux.Vars(r)
	paymentID := vars["id"]

	if err := h.store.FulfillPayment(r.Context(), paymentID); err != nil {
		sendError(w, http.StatusInternalServerError, err.Error())
		return
	}

	sendJSON(w, http.StatusOK, map[string]string{"status": "fulfilled"})
}

// GetOrderStatus retrieves the current status of a PoD order.
func (h *Handler) GetOrderStatus(w http.ResponseWriter, r *http.Request) {
	if err := requireAdminToken(r); err != nil {
		sendError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	vars := mux.Vars(r)
	paymentID := vars["payment_id"]

	// Get payment
	payment, err := h.store.GetPayment(r.Context(), paymentID)
	if err != nil {
		sendError(w, http.StatusNotFound, "Payment not found")
		return
	}

	// Check if payment has fulfillment result
	if payment.FulfillmentResult == nil || len(payment.FulfillmentResult) == 0 {
		sendError(w, http.StatusNotFound, "No fulfillment result for this payment")
		return
	}

	// Extract provider and order ID from fulfillment result
	providerName, _ := payment.FulfillmentResult["provider"].(string)
	orderID, _ := payment.FulfillmentResult["order_id"].(string)

	if providerName == "" || orderID == "" {
		sendError(w, http.StatusBadRequest, "Invalid fulfillment result: missing provider or order_id")
		return
	}

	// Get item to extract API key
	item, err := h.store.GetItem(r.Context(), payment.ItemID)
	if err != nil {
		sendError(w, http.StatusNotFound, "Item not found")
		return
	}

	apiKey, ok := item.BackendConfig["api_key"].(string)
	if !ok || apiKey == "" {
		sendError(w, http.StatusBadRequest, "Missing API key in item configuration")
		return
	}

	// Create provider instance
	provider, err := pod.NewProvider(providerName, apiKey)
	if err != nil {
		sendError(w, http.StatusBadRequest, fmt.Sprintf("Failed to create provider: %v", err))
		return
	}

	// Get order status from provider
	status, err := provider.GetStatus(r.Context(), orderID)
	if err != nil {
		sendError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to get order status: %v", err))
		return
	}

	// Update fulfillment result with latest status
	updatedResult := payment.FulfillmentResult
	updatedResult["status"] = status.Status
	updatedResult["tracking_url"] = status.TrackingURL
	updatedResult["shipping_date"] = status.ShippingDate
	updatedResult["last_updated"] = status.LastUpdated

	// Save updated fulfillment result
	if err := h.store.UpdateFulfillmentResult(r.Context(), paymentID, updatedResult); err != nil {
		log.Printf("Failed to update fulfillment result: %v", err)
		// Continue anyway to return the status
	}

	sendJSON(w, http.StatusOK, updatedResult)
}

// CreateCategory creates a new category.
func (h *Handler) CreateCategory(w http.ResponseWriter, r *http.Request) {
	if err := requireAdminToken(r); err != nil {
		sendError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, http.StatusBadRequest, "Invalid request format")
		return
	}

	category, err := h.store.CreateCategory(r.Context(), req.Name, req.Description)
	if err != nil {
		sendError(w, http.StatusInternalServerError, err.Error())
		return
	}

	sendJSON(w, http.StatusCreated, category)
}

// ListCategories lists all categories.
func (h *Handler) ListCategories(w http.ResponseWriter, r *http.Request) {
	h.handleList(w, r, func(ctx context.Context) (interface{}, error) {
		return h.store.ListCategories(ctx)
	})
}

// UpdateCategory updates a category.
func (h *Handler) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	if err := requireAdminToken(r); err != nil {
		sendError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	vars := mux.Vars(r)
	id := vars["id"]

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Order       int    `json:"order"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, http.StatusBadRequest, "Invalid request format")
		return
	}

	updates := make(map[string]interface{})
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Description != "" {
		updates["description"] = req.Description
	}
	if req.Order != 0 {
		updates["order"] = req.Order
	}

	if err := h.store.UpdateCategory(r.Context(), id, updates); err != nil {
		sendError(w, http.StatusInternalServerError, err.Error())
		return
	}

	sendJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// DeleteCategory deletes a category.
func (h *Handler) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	h.handleDelete(w, r, h.store.DeleteCategory)
}

// CreateItem creates a new item.
func (h *Handler) CreateItem(w http.ResponseWriter, r *http.Request) {
	if err := requireAdminToken(r); err != nil {
		sendError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req struct {
		CategoryID    string         `json:"category_id"`
		Name          string         `json:"name"`
		Description   string         `json:"description"`
		Price         string         `json:"price"`
		Currency      string         `json:"currency"`
		BackendType   string         `json:"backend_type"`
		BackendConfig models.JSONMap `json:"backend_config"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, http.StatusBadRequest, "Invalid request format")
		return
	}

	item := models.NewItem(req.CategoryID, req.Name, req.Description, req.Price, req.Currency, req.BackendType)
	item.BackendConfig = req.BackendConfig

	createdItem, err := h.store.CreateItem(r.Context(), item)
	if err != nil {
		sendError(w, http.StatusBadRequest, err.Error())
		return
	}

	sendJSON(w, http.StatusCreated, createdItem)
}

// ListItems lists all items.
func (h *Handler) ListItems(w http.ResponseWriter, r *http.Request) {
	if err := requireAdminToken(r); err != nil {
		sendError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// Parse query parameters for filtering
	filters := make(map[string]interface{})
	if categoryID := r.URL.Query().Get("category_id"); categoryID != "" {
		filters["category_id"] = categoryID
	}
	if backendType := r.URL.Query().Get("backend_type"); backendType != "" {
		filters["backend_type"] = backendType
	}
	if active := r.URL.Query().Get("active"); active != "" {
		filters["active"] = active == "true"
	}

	items, err := h.store.ListItems(r.Context(), filters)
	if err != nil {
		sendError(w, http.StatusInternalServerError, err.Error())
		return
	}

	sendJSON(w, http.StatusOK, items)
}

// UpdateItem updates an item.
// updateItemRequest represents a request to update an item.
type updateItemRequest struct {
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	Price         string         `json:"price"`
	Currency      string         `json:"currency"`
	BackendType   string         `json:"backend_type"`
	BackendConfig models.JSONMap `json:"backend_config"`
	Active        *bool          `json:"active"`
}

func (h *Handler) UpdateItem(w http.ResponseWriter, r *http.Request) {
	if err := requireAdminToken(r); err != nil {
		sendError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	vars := mux.Vars(r)
	id := vars["id"]

	var req updateItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, http.StatusBadRequest, "Invalid request format")
		return
	}

	updates := buildItemUpdates(&req)
	if err := h.store.UpdateItem(r.Context(), id, updates); err != nil {
		sendError(w, http.StatusBadRequest, err.Error())
		return
	}

	sendJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// buildItemUpdates constructs the updates map from the request fields.
func buildItemUpdates(req *updateItemRequest) map[string]interface{} {
	updates := make(map[string]interface{})

	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Description != "" {
		updates["description"] = req.Description
	}
	if req.Price != "" {
		updates["price"] = req.Price
	}
	if req.Currency != "" {
		updates["currency"] = req.Currency
	}
	if req.BackendType != "" {
		updates["backend_type"] = req.BackendType
	}
	if req.BackendConfig != nil {
		updates["backend_config"] = req.BackendConfig
	}
	if req.Active != nil {
		updates["active"] = *req.Active
	}

	return updates
}

// DeleteItem deletes an item.
func (h *Handler) DeleteItem(w http.ResponseWriter, r *http.Request) {
	h.handleDelete(w, r, h.store.DeleteItem)
}

// CreateTag creates a new tag.
func (h *Handler) CreateTag(w http.ResponseWriter, r *http.Request) {
	if err := requireAdminToken(r); err != nil {
		sendError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req struct {
		Name string `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, http.StatusBadRequest, "Invalid request format")
		return
	}

	tag, err := h.store.CreateTag(r.Context(), req.Name)
	if err != nil {
		sendError(w, http.StatusInternalServerError, err.Error())
		return
	}

	sendJSON(w, http.StatusCreated, tag)
}

// ListTags lists all tags.
func (h *Handler) ListTags(w http.ResponseWriter, r *http.Request) {
	h.handleList(w, r, func(ctx context.Context) (interface{}, error) {
		return h.store.ListTags(ctx)
	})
}

// UpdateTag updates a tag.
func (h *Handler) UpdateTag(w http.ResponseWriter, r *http.Request) {
	if err := requireAdminToken(r); err != nil {
		sendError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	vars := mux.Vars(r)
	id := vars["id"]

	var req struct {
		Name string `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, http.StatusBadRequest, "Invalid request format")
		return
	}

	updates := make(map[string]interface{})
	if req.Name != "" {
		updates["name"] = req.Name
	}

	if err := h.store.UpdateTag(r.Context(), id, updates); err != nil {
		sendError(w, http.StatusInternalServerError, err.Error())
		return
	}

	sendJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// DeleteTag deletes a tag.
func (h *Handler) DeleteTag(w http.ResponseWriter, r *http.Request) {
	h.handleDelete(w, r, h.store.DeleteTag)
}

// AddItemTag associates a tag with an item.
func (h *Handler) AddItemTag(w http.ResponseWriter, r *http.Request) {
	if err := requireAdminToken(r); err != nil {
		sendError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	vars := mux.Vars(r)
	itemID := vars["id"]

	var req struct {
		TagID string `json:"tag_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, http.StatusBadRequest, "Invalid request format")
		return
	}

	if err := h.store.AddItemTag(r.Context(), itemID, req.TagID); err != nil {
		sendError(w, http.StatusBadRequest, err.Error())
		return
	}

	sendJSON(w, http.StatusOK, map[string]string{"status": "added"})
}

// RemoveItemTag removes a tag from an item.
func (h *Handler) RemoveItemTag(w http.ResponseWriter, r *http.Request) {
	if err := requireAdminToken(r); err != nil {
		sendError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	vars := mux.Vars(r)
	itemID := vars["id"]
	tagID := vars["tag_id"]

	if err := h.store.RemoveItemTag(r.Context(), itemID, tagID); err != nil {
		sendError(w, http.StatusBadRequest, err.Error())
		return
	}

	sendJSON(w, http.StatusOK, map[string]string{"status": "removed"})
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

	// Confirm payment
	if err := h.store.ConfirmPayment(ctx, payment.ID, payload.PaymentHash); err != nil {
		return fmt.Errorf("failed to confirm payment %s: %w", payment.ID, err)
	}

	log.Printf("Payment %s confirmed via webhook (invoice: %s, hash: %s)", payment.ID, payload.InvoiceID, payload.PaymentHash)

	// Auto-fulfill if enabled
	if h.shouldAutoFulfill() {
		if err := h.store.FulfillPayment(ctx, payment.ID); err != nil {
			log.Printf("Failed to auto-fulfill payment %s: %v", payment.ID, err)
			// Don't return error - payment is confirmed, fulfillment can be retried
		} else {
			log.Printf("Payment %s auto-fulfilled", payment.ID)
		}
	}

	return nil
}

// Helper functions

// handleList is a generic handler for listing resources.
func (h *Handler) handleList(w http.ResponseWriter, r *http.Request, listFn func(ctx context.Context) (interface{}, error)) {
	if err := requireAdminToken(r); err != nil {
		sendError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	results, err := listFn(r.Context())
	if err != nil {
		sendError(w, http.StatusInternalServerError, err.Error())
		return
	}

	sendJSON(w, http.StatusOK, results)
}

// handleDelete is a generic handler for delete operations.
func (h *Handler) handleDelete(w http.ResponseWriter, r *http.Request, deleteFn func(ctx context.Context, id string) error) {
	if err := requireAdminToken(r); err != nil {
		sendError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	vars := mux.Vars(r)
	id := vars["id"]

	if err := deleteFn(r.Context(), id); err != nil {
		sendError(w, http.StatusInternalServerError, err.Error())
		return
	}

	sendJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func sendJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func sendError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(JSONResponse{
		Success: false,
		Error:   message,
	})
}
