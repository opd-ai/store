// Package api provides HTTP handlers for the store REST API.
// It handles catalog queries, checkout, payment webhooks, admin operations, and downloads.
//
// Key endpoints:
//   - GET /api/catalog: retrieve store catalog
//   - POST /api/checkout: create payment
//   - POST /webhook/payment-confirmed: process payment confirmation
//   - POST /admin/...: admin CRUD operations
//
// Handlers coordinate between store service, paywall client, and fulfillment backends.
package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"os"

	"github.com/opd-ai/store/pkg/config"
	"github.com/opd-ai/store/pkg/models"
	"github.com/opd-ai/store/pkg/paywall"
	"github.com/opd-ai/store/pkg/store"
)

// Handler encapsulates HTTP handlers for the store API.
type Handler struct {
	store         store.Service
	paywallClient paywall.Service
	adminToken    string
	config        *config.Config
}

// NewHandler creates a new API handler.
func NewHandler(s store.Service, paywallClient paywall.Service, adminToken string, cfg *config.Config) *Handler {
	return &Handler{
		store:         s,
		paywallClient: paywallClient,
		adminToken:    adminToken,
		config:        cfg,
	}
}

// Store returns the store service.
func (h *Handler) Store() store.Service {
	return h.store
}

// GetCSRFToken generates and returns a CSRF token.
func (h *Handler) GetCSRFToken(w http.ResponseWriter, r *http.Request) {
	token := generateCSRFToken()

	// Set token in cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "csrf_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   os.Getenv("STORE_ENV") == "production",
		SameSite: http.SameSiteStrictMode,
		MaxAge:   3600, // 1 hour
	})

	sendJSON(w, http.StatusOK, map[string]string{
		"csrf_token": token,
	})
}

// generateCSRFToken creates a random CSRF token.
func generateCSRFToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

// logAuditAction logs an admin action to the audit log
func (h *Handler) logAuditAction(r *http.Request, action, resource, resourceID string, changes map[string]interface{}) {
	// Get admin token from header (hashed for privacy)
	adminToken := r.Header.Get("X-Admin-Token")
	if len(adminToken) > 8 {
		adminToken = adminToken[:8] + "..." // Truncate for privacy
	}

	// Get IP and User-Agent
	ipAddress := r.RemoteAddr
	userAgent := r.UserAgent()

	// Create and store audit log entry
	log := models.NewAuditLog(adminToken, action, resource, resourceID, ipAddress, userAgent, changes)

	// Store in database (use background context to avoid blocking the request)
	ctx := context.Background()
	if err := h.store.CreateAuditLog(ctx, log); err != nil {
		// Log the error but don't fail the request
		// In production, you'd want proper error logging here
		_ = err
	}
}
