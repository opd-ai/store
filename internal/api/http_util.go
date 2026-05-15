package api

import (
	"context"
	"encoding/json"
	"log"
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

// handleDelete is a generic handler for delete operations with audit logging.
func (h *Handler) handleDelete(w http.ResponseWriter, r *http.Request, deleteFn func(ctx context.Context, id string) error, resource string) {
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

	h.logAuditEvent(r, "delete_"+resource, resource, id, nil)

	sendJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// logAuditEvent creates an audit log entry for an admin action.
func (h *Handler) logAuditEvent(r *http.Request, action, resource, resourceID string, changes models.JSONMap) {
	adminToken := r.Header.Get("X-Admin-Token")
	ip := getClientIP(r)
	userAgent := r.UserAgent()

	auditLog := models.NewAuditLog(adminToken, action, resource, resourceID, ip, userAgent, changes)

	// Log in background to avoid blocking the response
	go func() {
		ctx := context.Background()
		if err := h.store.CreateAuditLog(ctx, auditLog); err != nil {
			// Log error but don't fail the request
			log.Printf("Failed to create audit log: %v", err)
		}
	}()
}
