// Package paywall provides integration with the embedded opd-ai/paywall library.
// This replaces the HTTP client approach with direct library integration.
package paywall

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	pw "github.com/opd-ai/paywall"
	"github.com/opd-ai/paywall/wallet"
)

// EmbeddedConfig holds configuration for the embedded paywall.
type EmbeddedConfig struct {
	TestNet               bool
	DBPath                string
	PaymentTimeout        time.Duration
	MinConfirmations      int
	MultisigEnabled       bool
	SellerPubKey          []byte
	ArbiterPubKey         []byte
	SellerPrivateKey      []byte
	EscrowTimeoutPhysical time.Duration
}

// EmbeddedPaywall wraps the direct paywall library for embedded integration.
type EmbeddedPaywall struct {
	config          *EmbeddedConfig
	paywall         *pw.Paywall
	escrowManager   *pw.EscrowManager
	multisigEnabled bool
}

// NewEmbeddedPaywall creates a new embedded paywall instance.
func NewEmbeddedPaywall(cfg EmbeddedConfig) (*EmbeddedPaywall, error) {
	if cfg.DBPath == "" {
		return nil, fmt.Errorf("paywall DB path is required")
	}

	// Validate multisig configuration if enabled
	if cfg.MultisigEnabled {
		if err := validateMultisigConfig(&cfg); err != nil {
			return nil, fmt.Errorf("invalid multisig config: %w", err)
		}
	}

	// Initialize actual paywall library instance
	pwConfig := &pw.Config{
		PriceInBTC:       0.0001, // Minimal price; actual price set per-payment in CreateInvoice
		TestNet:          cfg.TestNet,
		Store:            pw.NewFileStore(cfg.DBPath),
		PaymentTimeout:   cfg.PaymentTimeout,
		MinConfirmations: cfg.MinConfirmations,
	}

	if cfg.MultisigEnabled {
		pwConfig.MultisigEnabled = true
		pwConfig.MultisigRequired = 2
		pwConfig.MultisigTotal = 3
		// Buyer key will be provided per-payment when address is generated
		// Set up participant keys with seller and arbiter (buyer added later)
		pwConfig.ParticipantPubKeys = map[wallet.WalletType][][]byte{
			wallet.Bitcoin: {nil, cfg.SellerPubKey, cfg.ArbiterPubKey},
		}
		pwConfig.MultisigRole = pw.RoleSeller
		pwConfig.AuthorizedArbiters = [][]byte{cfg.ArbiterPubKey}

		// Set escrow timeout configuration
		if cfg.EscrowTimeoutPhysical > 0 {
			pwConfig.MinEscrowTimeout = 24 * time.Hour
			pwConfig.MaxEscrowTimeout = cfg.EscrowTimeoutPhysical
		}
	}

	paywallInstance, err := pw.NewPaywall(*pwConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize paywall: %w", err)
	}

	// Initialize escrow manager if multisig is enabled
	var escrowManager *pw.EscrowManager
	if cfg.MultisigEnabled {
		escrowManager, err = pw.NewEscrowManager(paywallInstance)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize escrow manager: %w", err)
		}
	}

	return &EmbeddedPaywall{
		config:          &cfg,
		paywall:         paywallInstance,
		escrowManager:   escrowManager,
		multisigEnabled: cfg.MultisigEnabled,
	}, nil
}

// validateMultisigConfig validates multisig configuration parameters.
func validateMultisigConfig(cfg *EmbeddedConfig) error {
	if len(cfg.SellerPubKey) == 0 {
		return fmt.Errorf("seller public key is required for multisig")
	}
	if len(cfg.ArbiterPubKey) == 0 {
		return fmt.Errorf("arbiter public key is required for multisig")
	}
	if len(cfg.SellerPrivateKey) == 0 {
		return fmt.Errorf("seller private key is required for multisig")
	}
	return nil
}

// CreateInvoice creates a payment invoice (implements Service interface for backward compatibility).
func (e *EmbeddedPaywall) CreateInvoice(ctx context.Context, amount, currency, callbackURL string) (*Invoice, error) {
	// For backward compatibility with existing code, we create an invoice in the old format
	// This is a simplified implementation that would need to interact with the actual paywall library

	paymentID := generatePaymentID()
	address := generateAddress(currency, e.config.TestNet)
	expiresAt := time.Now().Add(e.config.PaymentTimeout)

	return &Invoice{
		InvoiceID:      paymentID,
		Status:         "pending",
		PaymentAddress: address,
		QRCode:         generateQRCode(address, amount),
		ExpiresAt:      expiresAt,
	}, nil
}

// GetInvoiceStatus retrieves the status of a payment invoice.
func (e *EmbeddedPaywall) GetInvoiceStatus(ctx context.Context, invoiceID string) (*InvoiceStatus, error) {
	// TODO: Query actual paywall library for payment status
	return &InvoiceStatus{
		InvoiceID: invoiceID,
		Status:    "pending",
		Confirmed: false,
		Amount:    "",
		Currency:  "BTC",
	}, nil
}

// VerifyWebhook verifies webhook signatures (not needed for embedded mode).
func (e *EmbeddedPaywall) VerifyWebhook(signature string, payload []byte, secret string) (bool, error) {
	// Embedded mode doesn't use webhooks, so this is a no-op
	return true, nil
}

// CreateEmbeddedPayment creates a payment with optional escrow support.
func (e *EmbeddedPaywall) CreateEmbeddedPayment(ctx context.Context, amount float64, timeout time.Duration, useEscrow bool) (*EmbeddedPayment, error) {
	if useEscrow && !e.multisigEnabled {
		return nil, fmt.Errorf("escrow requested but multisig not enabled")
	}

	paymentID := generatePaymentID()
	address := generateAddress("BTC", e.config.TestNet)
	expiresAt := time.Now().Add(timeout)

	payment := &EmbeddedPayment{
		ID:            paymentID,
		Address:       address,
		Amount:        amount,
		Currency:      "BTC",
		Status:        "pending",
		EscrowEnabled: useEscrow,
		RequiredSigs:  2,
		ExpiresAt:     expiresAt,
		Signatures:    []SignatureData{},
	}

	if useEscrow {
		payment.EscrowState = "created"
	}

	return payment, nil
}

// ConfirmEmbeddedPayment marks a payment as confirmed.
func (e *EmbeddedPaywall) ConfirmEmbeddedPayment(ctx context.Context, paymentID string, txHash string) error {
	// TODO: Update payment status in paywall library
	return nil
}

// GetEmbeddedPayment retrieves a payment by ID.
func (e *EmbeddedPaywall) GetEmbeddedPayment(ctx context.Context, paymentID string) (*EmbeddedPayment, error) {
	// TODO: Query paywall library
	return nil, fmt.Errorf("payment not found: %s", paymentID)
}

// ReleaseEscrow releases escrowed funds to the seller.
func (e *EmbeddedPaywall) ReleaseEscrow(ctx context.Context, paymentID string, signatures []SignatureData) error {
	if !e.multisigEnabled || e.escrowManager == nil {
		return fmt.Errorf("escrow not enabled")
	}

	if len(signatures) < 2 {
		return fmt.Errorf("insufficient signatures: need 2, got %d", len(signatures))
	}

	// Convert to paywall library signature format and identify buyer/seller signatures
	var buyerSig, sellerSig *pw.SignatureData
	for i := range signatures {
		pwSig := &pw.SignatureData{
			SignerID:  signatures[i].SignerID,
			Role:      pw.MultisigRole(signatures[i].Role),
			Signature: signatures[i].Signature,
			PublicKey: signatures[i].PublicKey,
			SignedAt:  signatures[i].SignedAt,
		}

		if signatures[i].Role == "buyer" {
			buyerSig = pwSig
		} else if signatures[i].Role == "seller" {
			sellerSig = pwSig
		}
	}

	if buyerSig == nil || sellerSig == nil {
		return fmt.Errorf("missing buyer or seller signature")
	}

	// Call escrow manager to release funds
	return e.escrowManager.ReleaseToSeller(paymentID, buyerSig, sellerSig)
}

// RefundEscrow refunds escrowed funds to the buyer.
func (e *EmbeddedPaywall) RefundEscrow(ctx context.Context, paymentID string, signatures []SignatureData) error {
	if !e.multisigEnabled || e.escrowManager == nil {
		return fmt.Errorf("escrow not enabled")
	}

	if len(signatures) < 2 {
		return fmt.Errorf("insufficient signatures: need 2, got %d", len(signatures))
	}

	// Convert to paywall library signature format
	var sig1, sig2 *pw.SignatureData
	for i := range signatures {
		pwSig := &pw.SignatureData{
			SignerID:  signatures[i].SignerID,
			Role:      pw.MultisigRole(signatures[i].Role),
			Signature: signatures[i].Signature,
			PublicKey: signatures[i].PublicKey,
			SignedAt:  signatures[i].SignedAt,
		}

		if i == 0 {
			sig1 = pwSig
		} else if i == 1 {
			sig2 = pwSig
		}
	}

	if sig1 == nil || sig2 == nil {
		return fmt.Errorf("need exactly 2 signatures")
	}

	// Call escrow manager to refund
	return e.escrowManager.RefundBuyer(paymentID, sig1, sig2)
}

// DisputeEscrow marks a payment as disputed.
func (e *EmbeddedPaywall) DisputeEscrow(ctx context.Context, paymentID string, reason string) error {
	if !e.multisigEnabled || e.escrowManager == nil {
		return fmt.Errorf("escrow not enabled")
	}

	// Request dispute as seller (this store acts as seller)
	return e.escrowManager.RequestDispute(paymentID, pw.RoleSeller, reason)
}

// ResolveDispute resolves a disputed escrow payment.
func (e *EmbeddedPaywall) ResolveDispute(ctx context.Context, paymentID string, resolution string, arbiterSig SignatureData) error {
	if !e.multisigEnabled || e.escrowManager == nil {
		return fmt.Errorf("escrow not enabled")
	}

	// TODO: This method signature needs to be extended to include the winner's signature
	// The paywall library's ResolveDispute requires both arbiter and winner signatures
	// For now, return an error indicating the limitation

	// Determine winner role based on resolution
	var winnerRole pw.MultisigRole
	if resolution == "release" {
		winnerRole = pw.RoleSeller
	} else if resolution == "refund" {
		winnerRole = pw.RoleBuyer
	} else {
		return fmt.Errorf("invalid resolution: %s (must be 'release' or 'refund')", resolution)
	}

	// Note: Full implementation requires extending the API to accept winner signature
	return fmt.Errorf("dispute resolution requires both arbiter signature and winner signature (role: %s) - API extension needed", winnerRole)
}

// Helper functions for payment generation

func generatePaymentID() string {
	return fmt.Sprintf("pay_%d", time.Now().UnixNano())
}

func generateAddress(currency string, testnet bool) string {
	// Simplified address generation (would use actual wallet in production)
	prefix := "1" // Bitcoin mainnet
	if testnet {
		prefix = "m" // Bitcoin testnet
	}
	return fmt.Sprintf("%s%s", prefix, generateRandomString(33))
}

func generateRandomString(length int) string {
	// Simplified random string (would use crypto/rand in production)
	return hex.EncodeToString([]byte(time.Now().String()))[:length]
}

func generateQRCode(address, amount string) string {
	// Return a placeholder QR code data URL
	return fmt.Sprintf("bitcoin:%s?amount=%s", address, amount)
}

// DecodeKey decodes a hex-encoded key.
func DecodeKey(keyHex string) ([]byte, error) {
	if keyHex == "" {
		return nil, nil
	}
	return hex.DecodeString(keyHex)
}

// DecryptKey decrypts an encrypted key (placeholder for actual encryption).
func DecryptKey(encryptedKey string, encryptionKey string) ([]byte, error) {
	// TODO: Implement actual decryption
	// For now, assume keys are stored in hex format
	if encryptedKey == "" {
		return nil, nil
	}
	return hex.DecodeString(encryptedKey)
}
