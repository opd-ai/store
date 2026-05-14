package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Category represents an item category in the store.
type Category struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description string    `json:"description"`
	Order       int       `json:"order"`
	Metadata    JSONMap   `json:"metadata"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Items       []Item    `json:"items,omitempty"`
}

// Tag represents a searchable tag for items.
type Tag struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
	Items     []Item    `json:"items,omitempty"`
}

// Item represents a product in the store.
type Item struct {
	ID            string    `json:"id"`
	CategoryID    string    `json:"category_id"`
	Category      *Category `json:"category,omitempty"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	Price         string    `json:"price"` // uint64 satoshis or string for precision
	Currency      string    `json:"currency"`
	Image         string    `json:"image"`
	BackendType   string    `json:"backend_type"`
	BackendConfig JSONMap   `json:"backend_config"`
	Metadata      JSONMap   `json:"metadata"`
	Active        bool      `json:"active"`
	Tags          []Tag     `json:"tags,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Payment represents a transaction for an item.
type Payment struct {
	ID                string     `json:"id"`
	ItemID            string     `json:"item_id"`
	Item              *Item      `json:"item,omitempty"`
	InvoiceID         string     `json:"invoice_id"`
	PaymentHash       *string    `json:"payment_hash"`
	Status            string     `json:"status"` // pending, confirmed, failed, fulfilled
	PayerInfo         JSONMap    `json:"payer_info"`
	Amount            string     `json:"amount"`
	Currency          string     `json:"currency"`
	ConfirmedAt       *time.Time `json:"confirmed_at"`
	FulfilledAt       *time.Time `json:"fulfilled_at"`
	FulfillmentResult JSONMap    `json:"fulfillment_result"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// FormSubmission stores form data collected by fulfillment handlers.
type FormSubmission struct {
	ID          string     `json:"id"`
	PaymentID   string     `json:"payment_id"`
	Payment     *Payment   `json:"payment,omitempty"`
	FormData    JSONMap    `json:"form_data"`
	Submitted   bool       `json:"submitted"`
	ProcessedAt *time.Time `json:"processed_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

// DownloadLog tracks download attempts for rate limiting.
type DownloadLog struct {
	ID           string    `json:"id"`
	PaymentID    string    `json:"payment_id"`
	Payment      *Payment  `json:"payment,omitempty"`
	IPAddress    string    `json:"ip_address"`
	UserAgent    string    `json:"user_agent"`
	DownloadedAt time.Time `json:"downloaded_at"`
}

// JSONMap custom type for JSON fields.
type JSONMap map[string]interface{}

// MarshalJSON implements json.Marshaler interface.
func (j JSONMap) MarshalJSON() ([]byte, error) {
	if j == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(map[string]interface{}(j))
}

// UnmarshalJSON implements json.Unmarshaler interface.
func (j *JSONMap) UnmarshalJSON(data []byte) error {
	var jsonMap map[string]interface{}
	if err := json.Unmarshal(data, &jsonMap); err != nil {
		return err
	}
	*j = JSONMap(jsonMap)
	return nil
}

// NewID generates a new UUID.
func NewID() string {
	return uuid.New().String()
}

// NewCategory creates a new category with defaults.
func NewCategory(name, description string) *Category {
	return &Category{
		ID:          NewID(),
		Name:        name,
		Description: description,
		Metadata:    JSONMap{},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

// NewTag creates a new tag.
func NewTag(name string) *Tag {
	return &Tag{
		ID:        NewID(),
		Name:      name,
		CreatedAt: time.Now(),
	}
}

// NewItem creates a new item.
func NewItem(categoryID, name, description, price, currency, backendType string) *Item {
	return &Item{
		ID:            NewID(),
		CategoryID:    categoryID,
		Name:          name,
		Description:   description,
		Price:         price,
		Currency:      currency,
		BackendType:   backendType,
		BackendConfig: JSONMap{},
		Metadata:      JSONMap{},
		Active:        true,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
}

// NewPayment creates a new payment record.
func NewPayment(itemID, amount, currency string) *Payment {
	return &Payment{
		ID:        NewID(),
		ItemID:    itemID,
		Status:    "pending",
		Amount:    amount,
		Currency:  currency,
		PayerInfo: JSONMap{},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

// Confirm marks a payment as confirmed.
func (p *Payment) Confirm() {
	now := time.Now()
	p.ConfirmedAt = &now
	p.updateStatus("confirmed", now)
}

// Fulfill marks a payment as fulfilled.
func (p *Payment) Fulfill(result map[string]interface{}) {
	now := time.Now()
	p.FulfilledAt = &now
	p.FulfillmentResult = result
	p.updateStatus("fulfilled", now)
}

// updateStatus updates the payment status and timestamp.
func (p *Payment) updateStatus(status string, now time.Time) {
	p.Status = status
	p.UpdatedAt = now
}

// IsConfirmed returns true if payment is confirmed.
func (p *Payment) IsConfirmed() bool {
	return p.Status == "confirmed" || p.Status == "fulfilled"
}

// NewFormSubmission creates a new form submission.
func NewFormSubmission(paymentID string, formData map[string]interface{}) *FormSubmission {
	return &FormSubmission{
		ID:        NewID(),
		PaymentID: paymentID,
		FormData:  formData,
		CreatedAt: time.Now(),
	}
}

// NewDownloadLog creates a new download log entry.
func NewDownloadLog(paymentID, ipAddress, userAgent string) *DownloadLog {
	return &DownloadLog{
		ID:           NewID(),
		PaymentID:    paymentID,
		IPAddress:    ipAddress,
		UserAgent:    userAgent,
		DownloadedAt: time.Now(),
	}
}

// AuditLog represents an admin action audit log entry
type AuditLog struct {
	ID         string    `json:"id"`
	Timestamp  time.Time `json:"timestamp"`
	AdminToken string    `json:"admin_token"` // Hashed or truncated for privacy
	Action     string    `json:"action"`      // e.g., "create_item", "delete_category", "fulfill_payment"
	Resource   string    `json:"resource"`    // e.g., "item", "category", "payment"
	ResourceID string    `json:"resource_id"`
	Changes    JSONMap   `json:"changes,omitempty"` // Optional: what changed
	IPAddress  string    `json:"ip_address"`
	UserAgent  string    `json:"user_agent"`
}

// NewAuditLog creates a new audit log entry
func NewAuditLog(adminToken, action, resource, resourceID, ipAddress, userAgent string, changes JSONMap) *AuditLog {
	// Truncate admin token for privacy (only store first 8 chars)
	truncatedToken := adminToken
	if len(truncatedToken) > 8 {
		truncatedToken = truncatedToken[:8] + "..."
	}

	return &AuditLog{
		ID:         NewID(),
		Timestamp:  time.Now(),
		AdminToken: truncatedToken,
		Action:     action,
		Resource:   resource,
		ResourceID: resourceID,
		Changes:    changes,
		IPAddress:  ipAddress,
		UserAgent:  userAgent,
	}
}
