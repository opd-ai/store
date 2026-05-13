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

// doRequest performs an HTTP request to the Printful API and decodes the response.
// If result is nil, the response body is not decoded (useful for DELETE requests).
func (c *Client) doRequest(ctx context.Context, method, endpoint string, body, result interface{}) error {
	req, err := c.prepareRequest(ctx, method, endpoint, body)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	return c.processResponse(resp, result)
}

// prepareRequest creates and configures an HTTP request with body marshaling.
func (c *Client) prepareRequest(ctx context.Context, method, endpoint string, body interface{}) (*http.Request, error) {
	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request: %w", err)
		}
		reqBody = bytes.NewReader(jsonData)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+endpoint, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return req, nil
}

// processResponse reads and validates the Printful API response.
func (c *Client) processResponse(resp *http.Response, result interface{}) error {
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	var pfResp printfulResponse
	if err := json.Unmarshal(bodyBytes, &pfResp); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if err := c.checkResponseStatus(&pfResp, bodyBytes); err != nil {
		return err
	}

	// Decode result if provided
	if result != nil {
		if err := json.Unmarshal(pfResp.Result, result); err != nil {
			return fmt.Errorf("failed to decode result: %w", err)
		}
	}

	return nil
}

// checkResponseStatus validates Printful API status codes.
func (c *Client) checkResponseStatus(pfResp *printfulResponse, bodyBytes []byte) error {
	if pfResp.Code == 200 || pfResp.Code == 201 {
		return nil
	}

	if pfResp.Error != nil {
		return fmt.Errorf("printful API error (code %d): %s", pfResp.Error.Code, pfResp.Error.Message)
	}

	return fmt.Errorf("unexpected status code %d: %s", pfResp.Code, string(bodyBytes))
}

// CreateOrder creates a new order with Printful.
func (c *Client) CreateOrder(ctx context.Context, orderReq *OrderRequest) (*Order, error) {
	var order Order
	if err := c.doRequest(ctx, http.MethodPost, "/orders", orderReq, &order); err != nil {
		return nil, err
	}
	return &order, nil
}

// GetOrderStatus retrieves the status of an order.
func (c *Client) GetOrderStatus(ctx context.Context, orderID string) (*OrderStatus, error) {
	var status OrderStatus
	if err := c.doRequest(ctx, http.MethodGet, "/orders/"+orderID, nil, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

// CancelOrder cancels an existing order.
func (c *Client) CancelOrder(ctx context.Context, orderID string) error {
	return c.doRequest(ctx, http.MethodDelete, "/orders/"+orderID, nil, nil)
}
