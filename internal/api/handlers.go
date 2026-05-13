package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/mux"

	"github.com/opd-ai/store/pkg/models"
	"github.com/opd-ai/store/pkg/store"
)

// Handler encapsulates HTTP handlers for the store API.
type Handler struct {
	store *store.Store
}

// NewHandler creates a new API handler.
func NewHandler(s *store.Store) *Handler {
	return &Handler{store: s}
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

	sendJSON(w, http.StatusCreated, map[string]interface{}{
		"payment_id": payment.ID,
		"status":     payment.Status,
		"amount":     payment.Amount,
		"currency":   payment.Currency,
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

	response := map[string]interface{}{
		"id":                 payment.ID,
		"item_id":            payment.ItemID,
		"status":             payment.Status,
		"amount":             payment.Amount,
		"currency":           payment.Currency,
		"confirmed_at":       payment.ConfirmedAt,
		"fulfilled_at":       payment.FulfilledAt,
		"fulfillment_result": payment.FulfillmentResult,
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

	sendJSON(w, http.StatusOK, payments)
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

	category := models.NewCategory(req.Name, req.Description)
	// TODO: save to database

	sendJSON(w, http.StatusCreated, category)
}

// ListCategories lists all categories.
func (h *Handler) ListCategories(w http.ResponseWriter, r *http.Request) {
	if err := requireAdminToken(r); err != nil {
		sendError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// TODO: implement
	sendJSON(w, http.StatusOK, []models.Category{})
}

// UpdateCategory updates a category.
func (h *Handler) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	if err := requireAdminToken(r); err != nil {
		sendError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// TODO: implement
	sendJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// DeleteCategory deletes a category.
func (h *Handler) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	if err := requireAdminToken(r); err != nil {
		sendError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// TODO: implement
	sendJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
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
	// TODO: save to database

	sendJSON(w, http.StatusCreated, item)
}

// ListItems lists all items.
func (h *Handler) ListItems(w http.ResponseWriter, r *http.Request) {
	if err := requireAdminToken(r); err != nil {
		sendError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// TODO: implement
	sendJSON(w, http.StatusOK, []models.Item{})
}

// UpdateItem updates an item.
func (h *Handler) UpdateItem(w http.ResponseWriter, r *http.Request) {
	if err := requireAdminToken(r); err != nil {
		sendError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// TODO: implement
	sendJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// DeleteItem deletes an item.
func (h *Handler) DeleteItem(w http.ResponseWriter, r *http.Request) {
	if err := requireAdminToken(r); err != nil {
		sendError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// TODO: implement
	sendJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// Helper functions

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
