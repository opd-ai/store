package printful

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client represents a client for the Printful API.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// Order represents a Printful order.
type Order struct {
	OrderID      string    `json:"id"`
	ExternalID   string    `json:"external_id"`
	Status       string    `json:"status"`
	TrackingURL  string    `json:"tracking_url,omitempty"`
	ShippingDate string    `json:"shipping_date,omitempty"`
	Created      time.Time `json:"created"`
}

// OrderRequest represents a request to create an order.
type OrderRequest struct {
	Recipient    Recipient    `json:"recipient"`
	Items        []OrderItem  `json:"items"`
	RetailCosts  *RetailCosts `json:"retail_costs,omitempty"`
	ConfirmDraft bool         `json:"confirm"`
}

// Recipient represents shipping recipient information.
type Recipient struct {
	Name        string `json:"name"`
	Address1    string `json:"address1"`
	City        string `json:"city"`
	StateCode   string `json:"state_code,omitempty"`
	CountryCode string `json:"country_code"`
	Zip         string `json:"zip"`
	Email       string `json:"email,omitempty"`
	Phone       string `json:"phone,omitempty"`
}

// OrderItem represents a product in an order.
type OrderItem struct {
	VariantID int    `json:"variant_id"`
	Quantity  int    `json:"quantity"`
	Files     []File `json:"files,omitempty"`
}

// File represents a file for printing.
type File struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

// RetailCosts represents additional cost information.
type RetailCosts struct {
	Currency string `json:"currency"`
	Total    string `json:"total"`
}

// OrderStatus represents the status of an order.
type OrderStatus struct {
	OrderID      string `json:"id"`
	Status       string `json:"status"`
	TrackingURL  string `json:"tracking_url,omitempty"`
	ShippingDate string `json:"shipping_date,omitempty"`
}

// printfulResponse wraps Printful API responses.
type printfulResponse struct {
	Code   int             `json:"code"`
	Result json.RawMessage `json:"result"`
	Error  *printfulError  `json:"error,omitempty"`
}

// printfulError represents an error from the Printful API.
type printfulError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// NewClient creates a new Printful API client.
func NewClient(apiKey string) *Client {
	return &Client{
		baseURL: "https://api.printful.com",
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// CreateOrder creates a new order with Printful.
func (c *Client) CreateOrder(ctx context.Context, orderReq *OrderRequest) (*Order, error) {
	body, err := json.Marshal(orderReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal order request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/orders", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var pfResp printfulResponse
	if err := json.Unmarshal(bodyBytes, &pfResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if pfResp.Code != 200 && pfResp.Code != 201 {
		if pfResp.Error != nil {
			return nil, fmt.Errorf("printful API error (code %d): %s", pfResp.Error.Code, pfResp.Error.Message)
		}
		return nil, fmt.Errorf("unexpected status code %d: %s", pfResp.Code, string(bodyBytes))
	}

	var order Order
	if err := json.Unmarshal(pfResp.Result, &order); err != nil {
		return nil, fmt.Errorf("failed to decode order result: %w", err)
	}

	return &order, nil
}

// GetOrderStatus retrieves the status of an order.
func (c *Client) GetOrderStatus(ctx context.Context, orderID string) (*OrderStatus, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/orders/"+orderID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var pfResp printfulResponse
	if err := json.Unmarshal(bodyBytes, &pfResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if pfResp.Code != 200 {
		if pfResp.Error != nil {
			return nil, fmt.Errorf("printful API error (code %d): %s", pfResp.Error.Code, pfResp.Error.Message)
		}
		return nil, fmt.Errorf("unexpected status code %d: %s", pfResp.Code, string(bodyBytes))
	}

	var status OrderStatus
	if err := json.Unmarshal(pfResp.Result, &status); err != nil {
		return nil, fmt.Errorf("failed to decode order status result: %w", err)
	}

	return &status, nil
}

// CancelOrder cancels an existing order.
func (c *Client) CancelOrder(ctx context.Context, orderID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+"/orders/"+orderID, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	var pfResp printfulResponse
	if err := json.Unmarshal(bodyBytes, &pfResp); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if pfResp.Code != 200 {
		if pfResp.Error != nil {
			return fmt.Errorf("printful API error (code %d): %s", pfResp.Error.Code, pfResp.Error.Message)
		}
		return fmt.Errorf("unexpected status code %d: %s", pfResp.Code, string(bodyBytes))
	}

	return nil
}
