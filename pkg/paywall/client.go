// Package paywall provides integration with the embedded opd-ai/paywall library.
// It handles payment creation, confirmation, and escrow workflows for cryptocurrency payments.
//
// Supported cryptocurrencies: Bitcoin (BTC), Monero (XMR).
//
// Example usage:
//
//	cfg := paywall.EmbeddedConfig{TestNet: true, DBPath: "./paywall.db"}
//	pw, err := paywall.NewEmbeddedPaywall(cfg)
//	invoice, err := pw.CreateInvoice(ctx, amount, currency, "")
package paywall

import (
	"context"
	"time"
)

// Service defines the interface for paywall operations.
type Service interface {
	// Legacy methods for backward compatibility
	CreateInvoice(ctx context.Context, amount, currency, callbackURL string) (*Invoice, error)
	GetInvoiceStatus(ctx context.Context, invoiceID string) (*InvoiceStatus, error)
	VerifyWebhook(signature string, payload []byte, secret string) (bool, error)

	// New embedded paywall methods
	CreateEmbeddedPayment(ctx context.Context, amount float64, timeout time.Duration, useEscrow bool) (*EmbeddedPayment, error)
	ConfirmEmbeddedPayment(ctx context.Context, paymentID string, txHash string) error
	GetEmbeddedPayment(ctx context.Context, paymentID string) (*EmbeddedPayment, error)

	// Escrow-specific methods
	ReleaseEscrow(ctx context.Context, paymentID string, signatures []SignatureData) error
	RefundEscrow(ctx context.Context, paymentID string, signatures []SignatureData) error
	DisputeEscrow(ctx context.Context, paymentID string, reason string) error
	ResolveDispute(ctx context.Context, paymentID string, resolution string, arbiterSig SignatureData) error
}

// Verify that EmbeddedPaywall implements Service at compile time.
var _ Service = (*EmbeddedPaywall)(nil)

// Invoice represents a payment invoice.
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

// EmbeddedPayment represents a payment created with the embedded paywall.
type EmbeddedPayment struct {
	ID            string          `json:"id"`
	Address       string          `json:"address"`
	Amount        float64         `json:"amount"`
	Currency      string          `json:"currency"`
	Status        string          `json:"status"`
	EscrowEnabled bool            `json:"escrow_enabled"`
	EscrowState   string          `json:"escrow_state"`
	RequiredSigs  int             `json:"required_sigs"`
	Signatures    []SignatureData `json:"signatures"`
	ExpiresAt     time.Time       `json:"expires_at"`
	FundedAt      *time.Time      `json:"funded_at,omitempty"`
	ReleasedAt    *time.Time      `json:"released_at,omitempty"`
}

// SignatureData represents a signature in a multisig transaction.
type SignatureData struct {
	SignerID  string    `json:"signer_id"`
	Role      string    `json:"role"` // buyer, seller, arbiter
	Signature []byte    `json:"signature"`
	PublicKey []byte    `json:"public_key"`
	SignedAt  time.Time `json:"signed_at"`
}
