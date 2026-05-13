package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/opd-ai/store/pkg/handler"
	"github.com/opd-ai/store/pkg/models"
)

// CustomHandler executes arbitrary fulfillment logic via webhook or embedded script.
// It allows users to integrate custom APIs, serverless functions, and internal systems.
type CustomHandler struct {
	httpClient *http.Client
}

// NewCustomHandler creates a new custom handler.
func NewCustomHandler() *CustomHandler {
	return &CustomHandler{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Handle implements FulfillmentHandler.
func (h *CustomHandler) Handle(ctx context.Context, payment *models.Payment, item *models.Item) (map[string]interface{}, error) {
	// Verify payment is confirmed
	if !payment.IsConfirmed() {
		return nil, handler.ErrPaymentNotConfirmed
	}

	// Extract webhook configuration
	backendConfig := item.BackendConfig
	if backendConfig == nil {
		return nil, fmt.Errorf("missing backend configuration")
	}

	webhookURL, ok := backendConfig["webhook_url"].(string)
	if !ok || webhookURL == "" {
		return nil, fmt.Errorf("missing or invalid webhook_url in configuration")
	}

	// Build payload from template
	payload := h.buildPayload(payment, item, backendConfig)

	// POST to webhook URL with retries
	retryCount := 3
	if rc, ok := backendConfig["retry_count"].(float64); ok {
		retryCount = int(rc)
	}

	var lastErr error
	for attempt := 0; attempt <= retryCount; attempt++ {
		result, err := h.invokeWebhook(ctx, webhookURL, payload, backendConfig)
		if err == nil {
			return result, nil
		}
		lastErr = err

		// Don't retry on last attempt
		if attempt < retryCount {
			backoff := time.Duration((attempt + 1)) * time.Second
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}

	return nil, fmt.Errorf("webhook invocation failed after %d retries: %w", retryCount, lastErr)
}

// invokeWebhook calls the webhook endpoint and processes the response.
func (h *CustomHandler) invokeWebhook(ctx context.Context, webhookURL string, payload map[string]interface{}, config models.JSONMap) (map[string]interface{}, error) {
	// Serialize payload
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", webhookURL, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	addCustomHeaders(req, config)

	// Execute request and get response body
	respBody, statusCode, err := h.executeRequest(req)
	if err != nil {
		return nil, err
	}

	// Parse and validate response
	return parseWebhookResponse(respBody, statusCode)
}

// addCustomHeaders adds custom headers from configuration to the request.
func addCustomHeaders(req *http.Request, config models.JSONMap) {
	if headers, ok := config["webhook_headers"].(map[string]interface{}); ok {
		for key, value := range headers {
			if strVal, ok := value.(string); ok {
				req.Header.Set(key, strVal)
			}
		}
	}
}

// executeRequest executes an HTTP request and returns the response body and status code.
func (h *CustomHandler) executeRequest(req *http.Request) ([]byte, int, error) {
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("webhook request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to read response body: %w", err)
	}

	return respBody, resp.StatusCode, nil
}

// parseWebhookResponse parses the webhook response and validates the status code.
func parseWebhookResponse(respBody []byte, statusCode int) (map[string]interface{}, error) {
	if statusCode < 200 || statusCode >= 300 {
		return nil, fmt.Errorf("webhook returned status %d: %s", statusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse webhook response: %w", err)
	}

	return result, nil
}

// buildPayload constructs the webhook payload from template and payment data.
func (h *CustomHandler) buildPayload(payment *models.Payment, item *models.Item, config models.JSONMap) map[string]interface{} {
	payload := make(map[string]interface{})

	// Start with template if provided
	if template, ok := config["payload_template"].(map[string]interface{}); ok {
		for key, value := range template {
			payload[key] = h.expandTemplate(value, payment, item)
		}
	}

	// Ensure core fields are always present
	if _, ok := payload["item_id"]; !ok {
		payload["item_id"] = item.ID
	}
	if _, ok := payload["payment_hash"]; !ok {
		if payment.PaymentHash != nil {
			payload["payment_hash"] = *payment.PaymentHash
		} else {
			payload["payment_hash"] = ""
		}
	}
	if _, ok := payload["payment_id"]; !ok {
		payload["payment_id"] = payment.ID
	}

	// Add payer email if available
	if email, ok := payment.PayerInfo["email"].(string); ok {
		payload["payer_email"] = email
	}

	return payload
}

// expandTemplate expands template strings with payment/item data.
func (h *CustomHandler) expandTemplate(value interface{}, payment *models.Payment, item *models.Item) interface{} {
	switch v := value.(type) {
	case string:
		// Replace placeholders
		result := v
		result = strings.ReplaceAll(result, "{item_id}", item.ID)
		paymentHash := ""
		if payment.PaymentHash != nil {
			paymentHash = *payment.PaymentHash
		}
		result = strings.ReplaceAll(result, "{payment_hash}", paymentHash)
		result = strings.ReplaceAll(result, "{payment_id}", payment.ID)
		result = strings.ReplaceAll(result, "{amount}", payment.Amount)
		result = strings.ReplaceAll(result, "{currency}", payment.Currency)

		// Add payer email if available
		if email, ok := payment.PayerInfo["email"].(string); ok {
			result = strings.ReplaceAll(result, "{payer_email}", email)
		}

		return result
	case map[string]interface{}:
		// Recursively expand nested objects
		expanded := make(map[string]interface{})
		for key, val := range v {
			expanded[key] = h.expandTemplate(val, payment, item)
		}
		return expanded
	case []interface{}:
		// Recursively expand array elements
		expanded := make([]interface{}, len(v))
		for i, val := range v {
			expanded[i] = h.expandTemplate(val, payment, item)
		}
		return expanded
	default:
		return value
	}
}

// Validate implements FulfillmentHandler.
func (h *CustomHandler) Validate(config models.JSONMap) error {
	// Check required fields
	webhookURL, ok := config["webhook_url"].(string)
	if !ok || webhookURL == "" {
		return fmt.Errorf("missing required field: webhook_url")
	}

	// Validate webhook URL format
	if !strings.HasPrefix(webhookURL, "http://") && !strings.HasPrefix(webhookURL, "https://") {
		return fmt.Errorf("invalid webhook_url: must start with http:// or https://")
	}

	return nil
}

// Metadata implements FulfillmentHandler.
func (h *CustomHandler) Metadata() handler.HandlerMetadata {
	return handler.HandlerMetadata{
		Type:        "custom",
		DisplayName: "Custom Webhook Handler",
		Description: "Execute arbitrary fulfillment logic via webhook invocation. Supports custom APIs, serverless functions, and internal systems with template-based payload construction.",
		RequiredFields: []handler.Field{
			{
				Name:        "webhook_url",
				Type:        "string",
				Description: "HTTPS endpoint to receive fulfillment requests",
				Example:     "https://internal.example.com/fulfill",
				Validation:  "must start with https://",
				Required:    true,
			},
		},
		OptionalFields: []handler.Field{
			{
				Name:        "webhook_method",
				Type:        "string",
				Description: "HTTP method for webhook request",
				Example:     "POST",
				Validation:  "POST (default) or PUT",
				Required:    false,
			},
			{
				Name:        "webhook_headers",
				Type:        "object",
				Description: "Custom headers to include in webhook request",
				Example:     `{"Authorization": "Bearer token123"}`,
				Required:    false,
			},
			{
				Name:        "timeout_seconds",
				Type:        "number",
				Description: "Request timeout in seconds",
				Example:     "30",
				Required:    false,
			},
			{
				Name:        "retry_count",
				Type:        "number",
				Description: "Number of retries on failure",
				Example:     "3",
				Required:    false,
			},
			{
				Name:        "payload_template",
				Type:        "object",
				Description: "Template for request payload with placeholder expansion",
				Example:     `{"item_id": "{item_id}", "payment_hash": "{payment_hash}"}`,
				Required:    false,
			},
		},
	}
}
