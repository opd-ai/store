// Package main provides a mock paywall service for local development and testing.
// This simulates the opd-ai/paywall API for cryptocurrency payment processing
// without requiring actual Bitcoin or Monero transactions.
package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

// Mock paywall service for testing
// Simulates opd-ai/paywall API responses

type PaymentRequest struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
	Callback string `json:"callback_url"`
}

type PaymentResponse struct {
	InvoiceID      string    `json:"invoice_id"`
	Status         string    `json:"status"`
	PaymentAddress string    `json:"payment_address"`
	QRCode         string    `json:"qr_code"`
	ExpiresAt      time.Time `json:"expires_at"`
}

type VerificationRequest struct {
	InvoiceID string `json:"invoice_id"`
	TxHash    string `json:"tx_hash"`
}

type VerificationResponse struct {
	InvoiceID string `json:"invoice_id"`
	Status    string `json:"status"`
	Confirmed bool   `json:"confirmed"`
	Amount    string `json:"amount"`
	Currency  string `json:"currency"`
}

// readJSONRequest validates the HTTP method and reads the request body.
// Returns the body bytes or writes an error response and returns nil.
func readJSONRequest(w http.ResponseWriter, r *http.Request) []byte {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return nil
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return nil
	}
	defer func() { _ = r.Body.Close() }()

	return body
}

func handleCreatePayment(w http.ResponseWriter, r *http.Request) {
	body := readJSONRequest(w, r)
	if body == nil {
		return
	}

	var req PaymentRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	// Generate mock invoice
	response := PaymentResponse{
		InvoiceID:      "inv_" + time.Now().Format("20060102150405"),
		Status:         "pending",
		PaymentAddress: "bc1qtest" + time.Now().Format("20060102150405"),
		QRCode:         "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
		ExpiresAt:      time.Now().Add(30 * time.Minute),
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func handleVerifyPayment(w http.ResponseWriter, r *http.Request) {
	body := readJSONRequest(w, r)
	if body == nil {
		return
	}

	var req VerificationRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	// Mock verification: always confirm
	response := VerificationResponse{
		InvoiceID: req.InvoiceID,
		Status:    "confirmed",
		Confirmed: true,
		Amount:    "100000",
		Currency:  "BTC",
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func main() {
	port := os.Getenv("PAYWALL_PORT")
	if port == "" {
		port = "8081"
	}

	http.HandleFunc("/api/payment/create", handleCreatePayment)
	http.HandleFunc("/api/payment/verify", handleVerifyPayment)
	http.HandleFunc("/health", handleHealth)

	log.Printf("Mock Paywall service starting on port %s\n", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
