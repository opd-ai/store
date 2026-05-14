package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/opd-ai/store/pkg/models"
	"github.com/opd-ai/store/pkg/pod"
)

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

	filters := map[string]interface{}{}
	if status := r.URL.Query().Get("status"); status != "" {
		filters["status"] = status
	}
	if itemID := r.URL.Query().Get("item_id"); itemID != "" {
		filters["item_id"] = itemID
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

	payment := h.getPaymentOrError(w, r, "payment_id")
	if payment == nil {
		return
	}

	providerName, orderID, err := extractProviderAndOrderID(payment)
	if err != nil {
		sendError(w, http.StatusBadRequest, err.Error())
		return
	}

	item, err := h.store.GetItem(r.Context(), payment.ItemID)
	if err != nil {
		sendError(w, http.StatusNotFound, "Item not found")
		return
	}

	apiKey, err := getProviderAPIKey(item)
	if err != nil {
		sendError(w, http.StatusBadRequest, err.Error())
		return
	}

	provider, err := pod.NewProvider(providerName, apiKey)
	if err != nil {
		sendError(w, http.StatusBadRequest, fmt.Sprintf("Failed to create provider: %v", err))
		return
	}

	status, err := provider.GetStatus(r.Context(), orderID)
	if err != nil {
		sendError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to get order status: %v", err))
		return
	}

	updatedResult := h.updateFulfillmentWithStatus(r.Context(), payment, status, payment.ID)
	sendJSON(w, http.StatusOK, updatedResult)
}

// extractProviderAndOrderID extracts provider name and order ID from payment fulfillment result.
func extractProviderAndOrderID(payment *models.Payment) (string, string, error) {
	if payment.FulfillmentResult == nil || len(payment.FulfillmentResult) == 0 {
		return "", "", fmt.Errorf("no fulfillment result for this payment")
	}

	providerName, _ := payment.FulfillmentResult["provider"].(string)
	orderID, _ := payment.FulfillmentResult["order_id"].(string)

	if providerName == "" || orderID == "" {
		return "", "", fmt.Errorf("invalid fulfillment result: missing provider or order_id")
	}

	return providerName, orderID, nil
}

// getProviderAPIKey retrieves the API key from item backend configuration.
func getProviderAPIKey(item *models.Item) (string, error) {
	apiKey, ok := item.BackendConfig["api_key"].(string)
	if !ok || apiKey == "" {
		return "", fmt.Errorf("missing API key in item configuration")
	}
	return apiKey, nil
}

// updateFulfillmentWithStatus updates the fulfillment result with latest status from provider.
func (h *Handler) updateFulfillmentWithStatus(ctx context.Context, payment *models.Payment, status *pod.OrderStatusResponse, paymentID string) models.JSONMap {
	updatedResult := payment.FulfillmentResult
	updatedResult["status"] = status.Status
	updatedResult["tracking_url"] = status.TrackingURL
	updatedResult["shipping_date"] = status.ShippingDate
	updatedResult["last_updated"] = status.LastUpdated

	if err := h.store.UpdateFulfillmentResult(ctx, paymentID, updatedResult); err != nil {
		log.Printf("Failed to update fulfillment result: %v", err)
	}

	return updatedResult
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

// UpdateItem updates an item.
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

	addStringField(updates, "name", req.Name)
	addStringField(updates, "description", req.Description)
	addStringField(updates, "price", req.Price)
	addStringField(updates, "currency", req.Currency)
	addStringField(updates, "backend_type", req.BackendType)
	addMapField(updates, "backend_config", req.BackendConfig)
	addBoolPtrField(updates, "active", req.Active)

	return updates
}

// addStringField adds a string field to updates if it's non-empty.
func addStringField(updates map[string]interface{}, key, value string) {
	if value != "" {
		updates[key] = value
	}
}

// addMapField adds a map field to updates if it's non-nil.
func addMapField(updates map[string]interface{}, key string, value map[string]interface{}) {
	if value != nil {
		updates[key] = value
	}
}

// addBoolPtrField adds a boolean pointer field to updates if it's non-nil.
func addBoolPtrField(updates map[string]interface{}, key string, value *bool) {
	if value != nil {
		updates[key] = *value
	}
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
