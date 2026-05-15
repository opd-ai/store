package api

import (
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/opd-ai/store/pkg/models"
	"github.com/opd-ai/store/pkg/paywall"
)

// SubmitShippingAddress handles shipping address collection for escrow payments.
func (h *Handler) SubmitShippingAddress(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	paymentID := vars["id"]

	// Get payment
	payment, err := h.store.GetPayment(r.Context(), paymentID)
	if err != nil {
		sendError(w, http.StatusNotFound, "Payment not found")
		return
	}

	// Verify escrow payment
	if !payment.EscrowEnabled {
		sendError(w, http.StatusBadRequest, "Payment is not an escrow payment")
		return
	}

	// Verify correct state
	if payment.EscrowState != "funded" {
		sendError(w, http.StatusBadRequest, fmt.Sprintf("Invalid escrow state: %s (expected: funded)", payment.EscrowState))
		return
	}

	// Parse shipping address
	var shippingInfo map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&shippingInfo); err != nil {
		sendError(w, http.StatusBadRequest, "Invalid request format")
		return
	}

	// Validate required fields
	if err := validateShippingInfo(shippingInfo); err != nil {
		sendError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Update payment with shipping info and transition state
	if err := h.store.UpdateEscrowState(r.Context(), paymentID, "address_submitted", shippingInfo); err != nil {
		sendError(w, http.StatusInternalServerError, "Failed to update escrow state")
		return
	}

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"payment_id":   paymentID,
		"escrow_state": "address_submitted",
		"message":      "Shipping address received. Processing order.",
	})
}

// MarkAsShipped marks an escrow payment as shipped (seller action).
func (h *Handler) MarkAsShipped(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	paymentID := vars["id"]

	// Verify admin token (seller only)
	if !h.isAuthorizedAdmin(r) {
		sendError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// Get payment
	payment, err := h.store.GetPayment(r.Context(), paymentID)
	if err != nil {
		sendError(w, http.StatusNotFound, "Payment not found")
		return
	}

	// Verify escrow payment
	if !payment.EscrowEnabled {
		sendError(w, http.StatusBadRequest, "Payment is not an escrow payment")
		return
	}

	// Verify correct state
	if payment.EscrowState != "address_submitted" {
		sendError(w, http.StatusBadRequest, fmt.Sprintf("Invalid escrow state: %s (expected: address_submitted)", payment.EscrowState))
		return
	}

	// Parse tracking information
	var trackingInfo map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&trackingInfo); err != nil {
		sendError(w, http.StatusBadRequest, "Invalid request format")
		return
	}

	// Update fulfillment result with tracking info
	if err := h.store.UpdateFulfillmentResult(r.Context(), paymentID, trackingInfo); err != nil {
		sendError(w, http.StatusInternalServerError, "Failed to update tracking info")
		return
	}

	// Transition escrow state
	if err := h.store.UpdateEscrowState(r.Context(), paymentID, "shipped", nil); err != nil {
		sendError(w, http.StatusInternalServerError, "Failed to update escrow state")
		return
	}

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"payment_id":   paymentID,
		"escrow_state": "shipped",
		"message":      "Order marked as shipped",
	})
}

// ReleaseEscrow releases escrowed funds to the seller (buyer or arbiter action).
func (h *Handler) ReleaseEscrow(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	paymentID := vars["id"]

	// Get payment
	payment, err := h.store.GetPayment(r.Context(), paymentID)
	if err != nil {
		sendError(w, http.StatusNotFound, "Payment not found")
		return
	}

	// Verify escrow payment
	if !payment.EscrowEnabled {
		sendError(w, http.StatusBadRequest, "Payment is not an escrow payment")
		return
	}

	// Verify correct state
	if payment.EscrowState != "shipped" && payment.EscrowState != "disputed" {
		sendError(w, http.StatusBadRequest, fmt.Sprintf("Invalid escrow state: %s", payment.EscrowState))
		return
	}

	// Parse signatures
	var req struct {
		Signatures []models.EscrowSignature `json:"signatures"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, http.StatusBadRequest, "Invalid request format")
		return
	}

	// Validate signatures (need at least 2 for 2-of-3 multisig)
	if len(req.Signatures) < 2 {
		sendError(w, http.StatusBadRequest, "Insufficient signatures: need 2")
		return
	}

	// Convert to paywall signature format
	signatures := make([]paywall.SignatureData, len(req.Signatures))
	for i, sig := range req.Signatures {
		// Decode hex-encoded signature and public key
		sigBytes, err := hex.DecodeString(sig.Signature)
		if err != nil {
			sendError(w, http.StatusBadRequest, fmt.Sprintf("Invalid signature format: %v", err))
			return
		}
		pubKeyBytes, err := hex.DecodeString(sig.PublicKey)
		if err != nil {
			sendError(w, http.StatusBadRequest, fmt.Sprintf("Invalid public key format: %v", err))
			return
		}

		signatures[i] = paywall.SignatureData{
			SignerID:  sig.SignerID,
			Role:      sig.SignerRole,
			Signature: sigBytes,
			PublicKey: pubKeyBytes,
			SignedAt:  sig.SignedAt,
		}
	}

	// Verify signatures with paywall service and broadcast transaction
	if err := h.paywallClient.ReleaseEscrow(r.Context(), paymentID, signatures); err != nil {
		sendError(w, http.StatusBadRequest, fmt.Sprintf("Failed to release escrow: %v", err))
		return
	}

	// Update payment state (only after successful blockchain transaction)
	if err := h.store.UpdateEscrowState(r.Context(), paymentID, "released", nil); err != nil {
		// Log error - blockchain tx already broadcast, state update is secondary
		fmt.Printf("WARNING: Escrow released on blockchain but failed to update database state: %v\n", err)
		// Still return success to user since funds were released
	}

	// Store signatures
	if err := h.store.UpdateEscrowSignatures(r.Context(), paymentID, req.Signatures); err != nil {
		// Log error but don't fail - funds already released
		fmt.Printf("Failed to store signatures: %v\n", err)
	}

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"payment_id":   paymentID,
		"escrow_state": "released",
		"message":      "Funds released to seller",
	})
}

// RefundEscrow refunds escrowed funds to the buyer (seller or arbiter action).
func (h *Handler) RefundEscrow(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	paymentID := vars["id"]

	// Verify admin token (seller or arbiter)
	if !h.isAuthorizedAdmin(r) {
		sendError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// Get payment
	payment, err := h.store.GetPayment(r.Context(), paymentID)
	if err != nil {
		sendError(w, http.StatusNotFound, "Payment not found")
		return
	}

	// Verify escrow payment
	if !payment.EscrowEnabled {
		sendError(w, http.StatusBadRequest, "Payment is not an escrow payment")
		return
	}

	// Parse signatures
	var req struct {
		Signatures []models.EscrowSignature `json:"signatures"`
		Reason     string                   `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, http.StatusBadRequest, "Invalid request format")
		return
	}

	// Validate signatures
	if len(req.Signatures) < 2 {
		sendError(w, http.StatusBadRequest, "Insufficient signatures: need 2")
		return
	}

	// Convert to paywall signature format
	signatures := make([]paywall.SignatureData, len(req.Signatures))
	for i, sig := range req.Signatures {
		// Decode hex-encoded signature and public key
		sigBytes, err := hex.DecodeString(sig.Signature)
		if err != nil {
			sendError(w, http.StatusBadRequest, fmt.Sprintf("Invalid signature format: %v", err))
			return
		}
		pubKeyBytes, err := hex.DecodeString(sig.PublicKey)
		if err != nil {
			sendError(w, http.StatusBadRequest, fmt.Sprintf("Invalid public key format: %v", err))
			return
		}

		signatures[i] = paywall.SignatureData{
			SignerID:  sig.SignerID,
			Role:      sig.SignerRole,
			Signature: sigBytes,
			PublicKey: pubKeyBytes,
			SignedAt:  sig.SignedAt,
		}
	}

	// Verify signatures and execute refund with paywall service
	if err := h.paywallClient.RefundEscrow(r.Context(), paymentID, signatures); err != nil {
		sendError(w, http.StatusBadRequest, fmt.Sprintf("Failed to refund escrow: %v", err))
		return
	}

	// Update payment state (only after successful blockchain transaction)
	if err := h.store.UpdateEscrowState(r.Context(), paymentID, "refunded", nil); err != nil {
		// Log error - blockchain tx already broadcast, state update is secondary
		fmt.Printf("WARNING: Escrow refunded on blockchain but failed to update database state: %v\n", err)
		// Still return success to user since funds were refunded
	}

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"payment_id":   paymentID,
		"escrow_state": "refunded",
		"message":      "Funds refunded to buyer",
	})
}

// InitiateDispute initiates a dispute for an escrow payment (buyer action).
func (h *Handler) InitiateDispute(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	paymentID := vars["id"]

	// Get payment
	payment, err := h.store.GetPayment(r.Context(), paymentID)
	if err != nil {
		sendError(w, http.StatusNotFound, "Payment not found")
		return
	}

	// Verify escrow payment
	if !payment.EscrowEnabled {
		sendError(w, http.StatusBadRequest, "Payment is not an escrow payment")
		return
	}

	// Verify correct state (can only dispute shipped orders)
	if payment.EscrowState != "shipped" {
		sendError(w, http.StatusBadRequest, fmt.Sprintf("Cannot dispute in state: %s", payment.EscrowState))
		return
	}

	// Parse dispute reason
	var req struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, http.StatusBadRequest, "Invalid request format")
		return
	}

	if req.Reason == "" {
		sendError(w, http.StatusBadRequest, "Dispute reason is required")
		return
	}

	// Update payment state
	if err := h.store.UpdateEscrowDispute(r.Context(), paymentID, req.Reason); err != nil {
		sendError(w, http.StatusInternalServerError, "Failed to initiate dispute")
		return
	}

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"payment_id":   paymentID,
		"escrow_state": "disputed",
		"message":      "Dispute initiated. Arbiter will review.",
	})
}

// ResolveDispute resolves a disputed escrow payment (arbiter action).
func (h *Handler) ResolveDispute(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	paymentID := vars["id"]

	// Verify admin token (arbiter only)
	if !h.isAuthorizedAdmin(r) {
		sendError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// Get payment
	payment, err := h.store.GetPayment(r.Context(), paymentID)
	if err != nil {
		sendError(w, http.StatusNotFound, "Payment not found")
		return
	}

	// Verify escrow payment
	if !payment.EscrowEnabled {
		sendError(w, http.StatusBadRequest, "Payment is not an escrow payment")
		return
	}

	// Verify correct state
	if payment.EscrowState != "disputed" {
		sendError(w, http.StatusBadRequest, fmt.Sprintf("Invalid escrow state: %s (expected: disputed)", payment.EscrowState))
		return
	}

	// Parse resolution
	var req struct {
		Resolution string                   `json:"resolution"` // "release" or "refund"
		Comment    string                   `json:"comment"`
		Signatures []models.EscrowSignature `json:"signatures"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, http.StatusBadRequest, "Invalid request format")
		return
	}

	if req.Resolution != "release" && req.Resolution != "refund" {
		sendError(w, http.StatusBadRequest, "Resolution must be 'release' or 'refund'")
		return
	}

	// Store resolution comment
	if err := h.store.UpdateEscrowResolution(r.Context(), paymentID, req.Comment); err != nil {
		sendError(w, http.StatusInternalServerError, "Failed to store resolution")
		return
	}

	// Execute resolution
	targetState := "released"
	if req.Resolution == "refund" {
		targetState = "refunded"
	}

	if err := h.store.UpdateEscrowState(r.Context(), paymentID, targetState, nil); err != nil {
		sendError(w, http.StatusInternalServerError, "Failed to execute resolution")
		return
	}

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"payment_id":   paymentID,
		"escrow_state": targetState,
		"resolution":   req.Resolution,
		"message":      "Dispute resolved by arbiter",
	})
}

// Helper functions

func validateShippingInfo(info map[string]interface{}) error {
	requiredFields := []string{"address1", "city", "state", "postal_code", "country"}
	for _, field := range requiredFields {
		if _, ok := info[field]; !ok {
			return fmt.Errorf("missing required field: %s", field)
		}
		if str, ok := info[field].(string); !ok || str == "" {
			return fmt.Errorf("invalid value for field: %s", field)
		}
	}
	return nil
}

func (h *Handler) isAuthorizedAdmin(r *http.Request) bool {
	token := r.Header.Get("X-Admin-Token")
	if token == "" || h.adminToken == "" {
		return false
	}
	// Use constant-time comparison to prevent timing attacks
	return subtle.ConstantTimeCompare([]byte(token), []byte(h.adminToken)) == 1
}
