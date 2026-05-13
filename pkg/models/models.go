package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Category represents an item category in the store.
type Category struct {
	ID          string    `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name" gorm:"index"`
	Slug        string    `json:"slug" gorm:"uniqueIndex"`
	Description string    `json:"description"`
	Order       int       `json:"order"`
	Metadata    JSONMap   `json:"metadata" gorm:"type:jsonb"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Items       []Item    `json:"items,omitempty" gorm:"foreignKey:CategoryID"`
}

// Tag represents a searchable tag for items.
type Tag struct {
	ID        string    `json:"id" gorm:"primaryKey"`
	Name      string    `json:"name" gorm:"index"`
	Slug      string    `json:"slug" gorm:"uniqueIndex"`
	CreatedAt time.Time `json:"created_at"`
	Items     []Item    `json:"items,omitempty" gorm:"many2many:item_tags"`
}

// Item represents a product in the store.
type Item struct {
	ID            string    `json:"id" gorm:"primaryKey"`
	CategoryID    string    `json:"category_id" gorm:"index"`
	Category      *Category `json:"-" gorm:"foreignKey:CategoryID"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	Price         string    `json:"price"` // uint64 satoshis or string for precision
	Currency      string    `json:"currency"`
	Image         string    `json:"image"`
	BackendType   string    `json:"backend_type" gorm:"index"`
	BackendConfig JSONMap   `json:"backend_config" gorm:"type:jsonb"`
	Metadata      JSONMap   `json:"metadata" gorm:"type:jsonb"`
	Active        bool      `json:"active" gorm:"index"`
	Tags          []Tag     `json:"tags,omitempty" gorm:"many2many:item_tags"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Payment represents a transaction for an item.
type Payment struct {
	ID                string     `json:"id" gorm:"primaryKey"`
	ItemID            string     `json:"item_id" gorm:"index"`
	Item              *Item      `json:"-" gorm:"foreignKey:ItemID"`
	InvoiceID         string     `json:"invoice_id" gorm:"index"`
	PaymentHash       *string    `json:"payment_hash" gorm:"uniqueIndex"`
	Status            string     `json:"status" gorm:"index"` // pending, confirmed, failed, fulfilled
	PayerInfo         JSONMap    `json:"payer_info" gorm:"type:jsonb"`
	Amount            string     `json:"amount"`
	Currency          string     `json:"currency"`
	ConfirmedAt       *time.Time `json:"confirmed_at"`
	FulfilledAt       *time.Time `json:"fulfilled_at"`
	FulfillmentResult JSONMap    `json:"fulfillment_result" gorm:"type:jsonb"`
	CreatedAt         time.Time  `json:"created_at" gorm:"index"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// FormSubmission stores form data collected by fulfillment handlers.
type FormSubmission struct {
	ID          string     `json:"id" gorm:"primaryKey"`
	PaymentID   string     `json:"payment_id" gorm:"index"`
	Payment     *Payment   `json:"-" gorm:"foreignKey:PaymentID"`
	FormData    JSONMap    `json:"form_data" gorm:"type:jsonb"`
	Submitted   bool       `json:"submitted"`
	ProcessedAt *time.Time `json:"processed_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

// DownloadLog tracks download attempts for rate limiting.
type DownloadLog struct {
	ID           string    `json:"id" gorm:"primaryKey"`
	PaymentID    string    `json:"payment_id" gorm:"index"`
	Payment      *Payment  `json:"-" gorm:"foreignKey:PaymentID"`
	IPAddress    string    `json:"ip_address" gorm:"index"`
	UserAgent    string    `json:"user_agent"`
	DownloadedAt time.Time `json:"downloaded_at" gorm:"index"`
}

// JSONMap custom type for JSONB fields.
type JSONMap map[string]interface{}

// Scan implements sql.Scanner interface.
func (j *JSONMap) Scan(value interface{}) error {
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("type assertion to []byte failed: %v", value)
	}
	return json.Unmarshal(bytes, &j)
}

// Value implements driver.Valuer interface.
func (j JSONMap) Value() (driver.Value, error) {
	return json.Marshal(j)
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
	p.Status = "confirmed"
	p.UpdatedAt = now
}

// Fulfill marks a payment as fulfilled.
func (p *Payment) Fulfill(result map[string]interface{}) {
	now := time.Now()
	p.FulfilledAt = &now
	p.Status = "fulfilled"
	p.FulfillmentResult = result
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
