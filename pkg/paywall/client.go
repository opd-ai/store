package paywall

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	// DefaultHTTPTimeout is the default timeout for HTTP requests to the paywall service.
	DefaultHTTPTimeout = 30 * time.Second
)

// Service defines the interface for paywall operations.
type Service interface {
	// CreateInvoice creates a new payment invoice.
	CreateInvoice(ctx context.Context, amount, currency, callbackURL string) (*Invoice, error)

	// GetInvoiceStatus retrieves the status of a payment invoice.
	GetInvoiceStatus(ctx context.Context, invoiceID string) (*InvoiceStatus, error)

	// VerifyWebhook verifies a webhook signature.
	VerifyWebhook(signature string, payload []byte, secret string) (bool, error)
}

// Client represents a client for the opd-ai/paywall service.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// Verify that Client implements Service at compile time.
var _ Service = (*Client)(nil)

// Invoice represents a payment invoice from the paywall service.
type Invoice struct {
	InvoiceID      string    `json:"invoice_id"`
	Status         string    `json:"status"`
	PaymentAddress string    `json:"payment_address"`
	QRCode         string    `json:"qr_code"`
	ExpiresAt      time.Time `json:"expires_at"`
}

// InvoiceStatus represents the status of a payment invoice.
type InvoiceStatus struct {
	InvoiceID string `json:"invoice_id"`
	Status    string `json:"status"`
	Confirmed bool   `json:"confirmed"`
	Amount    string `json:"amount"`
	Currency  string `json:"currency"`
}

// NewClient creates a new paywall client.
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: DefaultHTTPTimeout,
		},
	}
}

// doRequest performs an HTTP POST request and decodes the JSON response.
func (c *Client) doRequest(ctx context.Context, endpoint string, reqBody, respBody interface{}) error {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(bodyBytes))
	}

	if err := json.NewDecoder(resp.Body).Decode(respBody); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	return nil
}

// CreateInvoice creates a new payment invoice.
func (c *Client) CreateInvoice(ctx context.Context, amount, currency, callbackURL string) (*Invoice, error) {
	reqBody := map[string]string{
		"amount":       amount,
		"currency":     currency,
		"callback_url": callbackURL,
	}

	var invoice Invoice
	if err := c.doRequest(ctx, "/create-payment", reqBody, &invoice); err != nil {
		return nil, err
	}

	return &invoice, nil
}

// GetInvoiceStatus retrieves the status of an invoice.
func (c *Client) GetInvoiceStatus(ctx context.Context, invoiceID string) (*InvoiceStatus, error) {
	reqBody := map[string]string{
		"invoice_id": invoiceID,
		"tx_hash":    "", // Empty for status check
	}

	var status InvoiceStatus
	if err := c.doRequest(ctx, "/verify-payment", reqBody, &status); err != nil {
		return nil, err
	}

	return &status, nil
}

// VerifyWebhook verifies the signature of a webhook payload.
func (c *Client) VerifyWebhook(signature string, payload []byte, secret string) (bool, error) {
	mac := hmac.New(sha256.New, []byte(secret))
	if _, err := mac.Write(payload); err != nil {
		return false, fmt.Errorf("failed to compute HMAC: %w", err)
	}

	expectedSignature := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(signature), []byte(expectedSignature)), nil
}
