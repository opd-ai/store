package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/opd-ai/store/pkg/models"
)

// JSONResponse is a wrapper for JSON responses.
type JSONResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// sendJSON writes a JSON response.
func sendJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// sendError writes a JSON error response.
func sendError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(JSONResponse{
		Success: false,
		Error:   message,
	})
}

// getResourceOrError is a generic helper to retrieve a resource by ID from URL vars.
func getResourceOrError[T any](w http.ResponseWriter, r *http.Request, varName string, getter func(context.Context, string) (*T, error), errMsg string) *T {
	vars := mux.Vars(r)
	id := vars[varName]

	resource, err := getter(r.Context(), id)
	if err != nil {
		sendError(w, http.StatusNotFound, errMsg)
		return nil
	}

	return resource
}

// getPaymentOrError retrieves a payment by ID from URL vars or sends a 404 error.
// Returns nil if the payment was not found (error already sent to client).
func (h *Handler) getPaymentOrError(w http.ResponseWriter, r *http.Request, varName string) *models.Payment {
	return getResourceOrError(w, r, varName, h.store.GetPayment, "Payment not found")
}

// getItemOrError retrieves an item by ID from URL vars or sends a 404 error.
// Returns nil if the item was not found (error already sent to client).
func (h *Handler) getItemOrError(w http.ResponseWriter, r *http.Request, varName string) *models.Item {
	return getResourceOrError(w, r, varName, h.store.GetItem, "Item not found")
}

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
