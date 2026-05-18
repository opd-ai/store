// Package paywall provides integration with the embedded opd-ai/paywall library.
// This replaces the HTTP client approach with direct library integration.
package paywall

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	pw "github.com/opd-ai/paywall"
	"github.com/opd-ai/paywall/wallet"
	"github.com/opd-ai/store/pkg/crypto"
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
	// Query paywall library for payment
	payment, err := e.paywall.Store.GetPayment(invoiceID)
	if err != nil {
		return nil, fmt.Errorf("payment not found: %w", err)
	}

	// Determine status based on confirmations
	status := "pending"
	confirmed := false

	if payment.Confirmations >= e.config.MinConfirmations {
		status = "confirmed"
		confirmed = true
	} else if time.Now().After(payment.ExpiresAt) {
		status = "expired"
	}

	// Get amount and currency from the payment
	// Default to Bitcoin, or use the first non-zero amount
	var amount string
	var currency string

	if btcAmount, ok := payment.Amounts[wallet.Bitcoin]; ok && btcAmount > 0 {
		amount = fmt.Sprintf("%.8f", btcAmount)
		currency = string(wallet.Bitcoin)
	} else if xmrAmount, ok := payment.Amounts[wallet.Monero]; ok && xmrAmount > 0 {
		amount = fmt.Sprintf("%.12f", xmrAmount)
		currency = string(wallet.Monero)
	} else {
		// No amount set, return empty
		amount = "0"
		currency = "BTC"
	}

	return &InvoiceStatus{
		InvoiceID: invoiceID,
		Status:    status,
		Confirmed: confirmed,
		Amount:    amount,
		Currency:  currency,
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
func (e *EmbeddedPaywall) ConfirmEmbeddedPayment(ctx context.Context, paymentID, txHash string) error {
	// Get the payment
	payment, err := e.paywall.Store.GetPayment(paymentID)
	if err != nil {
		return fmt.Errorf("payment not found: %w", err)
	}

	// Update payment status and transaction hash
	payment.Status = pw.StatusConfirmed
	payment.TransactionID = txHash
	payment.BroadcastedAt = time.Now()

	// Save the updated payment
	if err := e.paywall.Store.UpdatePayment(payment); err != nil {
		return fmt.Errorf("failed to update payment: %w", err)
	}

	return nil
}

// GetEmbeddedPayment retrieves a payment by ID.
func (e *EmbeddedPaywall) GetEmbeddedPayment(ctx context.Context, paymentID string) (*EmbeddedPayment, error) {
	payment, err := e.paywall.Store.GetPayment(paymentID)
	if err != nil {
		return nil, fmt.Errorf("payment not found: %s", paymentID)
	}

	// Get the primary address and amount (default to Bitcoin)
	var address string
	var amount float64
	var currency string

	if btcAddr, ok := payment.Addresses[wallet.Bitcoin]; ok {
		address = btcAddr
		amount = payment.Amounts[wallet.Bitcoin]
		currency = string(wallet.Bitcoin)
	} else if xmrAddr, ok := payment.Addresses[wallet.Monero]; ok {
		address = xmrAddr
		amount = payment.Amounts[wallet.Monero]
		currency = string(wallet.Monero)
	}

	// Determine overall payment status
	status := string(payment.Status)
	escrowState := ""
	if payment.MultisigEnabled {
		switch payment.EscrowState {
		case 0: // EscrowNone
			escrowState = "none"
		case 1: // EscrowPending
			escrowState = "pending"
		case 2: // EscrowFunded
			escrowState = "funded"
		case 3: // EscrowReleased
			escrowState = "released"
		case 4: // EscrowRefunded
			escrowState = "refunded"
		case 5: // EscrowDisputed
			escrowState = "disputed"
		case 6: // EscrowTimeout
			escrowState = "timeout"
		}
	}

	embeddedPayment := &EmbeddedPayment{
		ID:            payment.ID,
		Address:       address,
		Amount:        amount,
		Currency:      currency,
		Status:        status,
		EscrowEnabled: payment.MultisigEnabled,
		EscrowState:   escrowState,
		ExpiresAt:     payment.ExpiresAt,
	}

	if payment.MultisigEnabled {
		// Convert signatures from paywall format to embedded format
		var signatures []SignatureData
		for walletType, sigs := range payment.Signatures {
			for _, sig := range sigs {
				signatures = append(signatures, SignatureData{
					SignerID:  sig.SignerID,
					Role:      string(sig.Role),
					Signature: sig.Signature,
					PublicKey: sig.PublicKey,
					SignedAt:  sig.SignedAt,
				})
			}
			// Use first wallet type's signatures (Bitcoin preferred)
			if walletType == wallet.Bitcoin {
				break
			}
		}
		embeddedPayment.Signatures = signatures
		embeddedPayment.RequiredSigs = 2 // 2-of-3 multisig

		// Set funded/released timestamps if available
		if !payment.BroadcastedAt.IsZero() {
			embeddedPayment.FundedAt = &payment.BroadcastedAt
		}
		// ReleasedAt would need to be tracked separately in a real implementation
	}

	return embeddedPayment, nil
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
func (e *EmbeddedPaywall) DisputeEscrow(ctx context.Context, paymentID, reason string) error {
	if !e.multisigEnabled || e.escrowManager == nil {
		return fmt.Errorf("escrow not enabled")
	}

	// Request dispute as seller (this store acts as seller)
	return e.escrowManager.RequestDispute(paymentID, pw.RoleSeller, reason)
}

// ResolveDispute resolves a disputed escrow payment.
func (e *EmbeddedPaywall) ResolveDispute(ctx context.Context, paymentID, resolution string, arbiterSig, winnerSig SignatureData) error {
	if !e.multisigEnabled || e.escrowManager == nil {
		return fmt.Errorf("escrow not enabled")
	}

	// Determine winner role based on resolution
	var winnerRole pw.MultisigRole
	if resolution == "release" {
		winnerRole = pw.RoleSeller
	} else if resolution == "refund" {
		winnerRole = pw.RoleBuyer
	} else {
		return fmt.Errorf("invalid resolution: %s (must be 'release' or 'refund')", resolution)
	}

	// Validate signature roles match expected parties
	if arbiterSig.Role != "arbiter" {
		return fmt.Errorf("first signature must be from arbiter, got role: %s", arbiterSig.Role)
	}

	expectedWinnerRole := string(winnerRole)
	if winnerSig.Role != expectedWinnerRole {
		return fmt.Errorf("second signature must be from %s (winner), got role: %s", expectedWinnerRole, winnerSig.Role)
	}

	// Convert to paywall library signature format
	pwArbiterSig := &pw.SignatureData{
		SignerID:  arbiterSig.SignerID,
		Role:      pw.MultisigRole(arbiterSig.Role),
		Signature: arbiterSig.Signature,
		PublicKey: arbiterSig.PublicKey,
		SignedAt:  arbiterSig.SignedAt,
	}

	pwWinnerSig := &pw.SignatureData{
		SignerID:  winnerSig.SignerID,
		Role:      pw.MultisigRole(winnerSig.Role),
		Signature: winnerSig.Signature,
		PublicKey: winnerSig.PublicKey,
		SignedAt:  winnerSig.SignedAt,
	}

	// Call paywall library to resolve dispute
	if err := e.escrowManager.ResolveDispute(paymentID, pwArbiterSig, pwWinnerSig); err != nil {
		return fmt.Errorf("failed to resolve dispute: %w", err)
	}

	return nil
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

// DecryptKey decrypts an encrypted key using AES-256-GCM.
func DecryptKey(encryptedKey, encryptionKey string) ([]byte, error) {
	if encryptedKey == "" {
		return nil, nil
	}

	// If encryption key is not provided, assume the key is stored as hex
	if encryptionKey == "" {
		return hex.DecodeString(encryptedKey)
	}

	// Initialize encryption service
	enc, err := crypto.NewEncryptionServiceFromBase64(encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize decryption: %w", err)
	}

	// Decode the base64-encoded encrypted key
	ciphertext, err := base64.StdEncoding.DecodeString(encryptedKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decode encrypted key: %w", err)
	}

	// Decrypt the key
	decryptedKey, err := enc.Decrypt(ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	return decryptedKey, nil
}
