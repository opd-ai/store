# Handler Development Guide

## Overview

This guide walks you through creating custom fulfillment handlers for opd-ai/store. Handlers determine what happens after a payment is confirmed - whether that's sending an email, creating a shipping label, calling an external API, or anything else you can imagine.

## Table of Contents

1. [Understanding Handlers](#understanding-handlers)
2. [The FulfillmentHandler Interface](#the-fulfillmenthandler-interface)
3. [Example: Email Notification Handler](#example-email-notification-handler)
4. [Backend Configuration](#backend-configuration)
5. [Validation Best Practices](#validation-best-practices)
6. [Testing Your Handler](#testing-your-handler)
7. [Registering Your Handler](#registering-your-handler)
8. [Advanced Topics](#advanced-topics)

## Understanding Handlers

Handlers are the core extensibility mechanism in opd-ai/store. Each item in the catalog has a `backend_type` that determines which handler will fulfill the order after payment is confirmed.

### Built-in Handlers

opd-ai/store includes four handlers:

- **digital_media**: Immediate downloads with expiration and rate limiting
- **shipping_form**: Collect customer address for physical goods
- **pod**: Print-on-demand integration (Printful, Redbubble, TeeSpring)
- **custom**: External webhook for custom fulfillment workflows

### When to Create a Custom Handler

Create a custom handler when:
- You need integration with a service not supported by built-in handlers
- You have complex business logic that doesn't fit the webhook model
- You want to keep fulfillment logic in-process for performance or reliability
- You need direct access to the database or store service

## The FulfillmentHandler Interface

All handlers implement this interface from `pkg/handler/handler.go`:

```go
type FulfillmentHandler interface {
    // Handle performs the fulfillment action for a confirmed payment
    Handle(ctx context.Context, payment *models.Payment, item *models.Item) (models.JSONMap, error)
    
    // Validate checks if the backend_config for an item is valid
    Validate(config models.JSONMap) error
    
    // Metadata returns information about this handler for documentation
    Metadata() HandlerMetadata
}
```

### Method Responsibilities

**Handle()**
- Called after payment confirmation
- Performs the actual fulfillment action
- Returns result data to store in `payment.fulfillment_result`
- Must be idempotent (safe to call multiple times with same payment)

**Validate()**
- Called when creating or updating an item
- Checks that `backend_config` has required fields and valid values
- Returns error if configuration is invalid
- Prevents items from being created with broken configurations

**Metadata()**
- Returns handler name, description, and documentation
- Used by admin UI and `/admin/handlers` endpoint
- Helps users understand what the handler does and how to configure it

## Example: Email Notification Handler

Let's build a handler that sends an email notification when an order is fulfilled. This is a complete, production-ready example.

### Step 1: Create the Handler File

Create `internal/handlers/email_notification.go`:

```go
package handlers

import (
    "context"
    "fmt"
    "net/smtp"
    "time"

    "github.com/opd-ai/store/pkg/handler"
    "github.com/opd-ai/store/pkg/models"
)

// EmailNotificationHandler sends email notifications upon payment fulfillment.
type EmailNotificationHandler struct{}

// NewEmailNotificationHandler creates a new email notification handler.
func NewEmailNotificationHandler() *EmailNotificationHandler {
    return &EmailNotificationHandler{}
}

// Handle sends an email notification to the specified recipient.
func (h *EmailNotificationHandler) Handle(ctx context.Context, payment *models.Payment, item *models.Item) (models.JSONMap, error) {
    // Extract configuration
    recipientEmail, ok := item.BackendConfig["recipient_email"].(string)
    if !ok {
        return nil, fmt.Errorf("recipient_email not configured")
    }

    smtpHost, ok := item.BackendConfig["smtp_host"].(string)
    if !ok {
        smtpHost = "localhost"
    }

    smtpPort, ok := item.BackendConfig["smtp_port"].(float64)
    if !ok {
        smtpPort = 25
    }

    fromEmail, ok := item.BackendConfig["from_email"].(string)
    if !ok {
        fromEmail = "noreply@example.com"
    }

    // Optional authentication
    smtpUsername, _ := item.BackendConfig["smtp_username"].(string)
    smtpPassword, _ := item.BackendConfig["smtp_password"].(string)

    // Extract payer email
    payerEmail, _ := payment.PayerInfo["email"].(string)
    if payerEmail == "" {
        payerEmail = "unknown"
    }

    // Build email message
    subject := fmt.Sprintf("Order Confirmed: %s", item.Name)
    body := fmt.Sprintf(
        "Order Details:\n\n"+
            "Item: %s\n"+
            "Payment ID: %s\n"+
            "Amount: %s %s\n"+
            "Payer: %s\n"+
            "Confirmed: %s\n\n"+
            "Thank you for your purchase!",
        item.Name,
        payment.ID,
        payment.Amount,
        payment.Currency,
        payerEmail,
        time.Now().Format(time.RFC3339),
    )

    message := fmt.Sprintf(
        "From: %s\r\n"+
            "To: %s\r\n"+
            "Subject: %s\r\n"+
            "\r\n"+
            "%s\r\n",
        fromEmail,
        recipientEmail,
        subject,
        body,
    )

    // Send email
    addr := fmt.Sprintf("%s:%d", smtpHost, int(smtpPort))
    
    var auth smtp.Auth
    if smtpUsername != "" && smtpPassword != "" {
        auth = smtp.PlainAuth("", smtpUsername, smtpPassword, smtpHost)
    }

    err := smtp.SendMail(addr, auth, fromEmail, []string{recipientEmail}, []byte(message))
    if err != nil {
        return nil, fmt.Errorf("failed to send email: %w", err)
    }

    // Return fulfillment result
    return models.JSONMap{
        "email_sent":       true,
        "recipient":        recipientEmail,
        "sent_at":          time.Now().Format(time.RFC3339),
        "notification_type": "order_confirmation",
    }, nil
}

// Validate checks if the email configuration is valid.
func (h *EmailNotificationHandler) Validate(config models.JSONMap) error {
    // Required field: recipient_email
    recipientEmail, ok := config["recipient_email"].(string)
    if !ok || recipientEmail == "" {
        return fmt.Errorf("recipient_email is required and must be a valid email address")
    }

    // Validate email format (basic check)
    if !isValidEmail(recipientEmail) {
        return fmt.Errorf("recipient_email must be a valid email address")
    }

    // Optional fields validation
    if smtpHost, ok := config["smtp_host"].(string); ok {
        if smtpHost == "" {
            return fmt.Errorf("smtp_host cannot be empty if provided")
        }
    }

    if smtpPort, ok := config["smtp_port"].(float64); ok {
        if smtpPort < 1 || smtpPort > 65535 {
            return fmt.Errorf("smtp_port must be between 1 and 65535")
        }
    }

    if fromEmail, ok := config["from_email"].(string); ok {
        if fromEmail != "" && !isValidEmail(fromEmail) {
            return fmt.Errorf("from_email must be a valid email address")
        }
    }

    // Validate SMTP credentials (both must be present or both absent)
    hasUsername := config["smtp_username"] != nil && config["smtp_username"].(string) != ""
    hasPassword := config["smtp_password"] != nil && config["smtp_password"].(string) != ""
    
    if hasUsername != hasPassword {
        return fmt.Errorf("smtp_username and smtp_password must both be provided or both omitted")
    }

    return nil
}

// Metadata returns handler information.
func (h *EmailNotificationHandler) Metadata() handler.HandlerMetadata {
    return handler.HandlerMetadata{
        Type:        "email_notification",
        Name:        "Email Notification Handler",
        Description: "Sends email notifications when orders are fulfilled",
        ConfigSchema: map[string]interface{}{
            "recipient_email": map[string]string{
                "type":        "string",
                "required":    "true",
                "description": "Email address to receive notifications",
            },
            "smtp_host": map[string]string{
                "type":        "string",
                "required":    "false",
                "default":     "localhost",
                "description": "SMTP server hostname",
            },
            "smtp_port": map[string]string{
                "type":        "number",
                "required":    "false",
                "default":     "25",
                "description": "SMTP server port",
            },
            "from_email": map[string]string{
                "type":        "string",
                "required":    "false",
                "default":     "noreply@example.com",
                "description": "Sender email address",
            },
            "smtp_username": map[string]string{
                "type":        "string",
                "required":    "false",
                "description": "SMTP authentication username",
            },
            "smtp_password": map[string]string{
                "type":        "string",
                "required":    "false",
                "description": "SMTP authentication password",
            },
        },
        ExampleConfig: map[string]interface{}{
            "recipient_email": "admin@example.com",
            "smtp_host":       "smtp.gmail.com",
            "smtp_port":       587,
            "from_email":      "store@example.com",
            "smtp_username":   "store@example.com",
            "smtp_password":   "app_password_here",
        },
    }
}

// isValidEmail performs basic email validation.
func isValidEmail(email string) bool {
    // Basic check: contains @ and has characters before and after
    // For production, use a proper email validation library
    if len(email) < 3 {
        return false
    }
    
    atIndex := -1
    for i, c := range email {
        if c == '@' {
            if atIndex != -1 {
                return false // Multiple @ signs
            }
            atIndex = i
        }
    }
    
    return atIndex > 0 && atIndex < len(email)-1
}
```

### Step 2: Create Tests

Create `internal/handlers/email_notification_test.go`:

```go
package handlers

import (
    "context"
    "testing"
    "time"

    "github.com/opd-ai/store/pkg/models"
)

func TestEmailNotificationHandler_Validate(t *testing.T) {
    handler := NewEmailNotificationHandler()

    tests := []struct {
        name    string
        config  models.JSONMap
        wantErr bool
    }{
        {
            name: "valid minimal configuration",
            config: models.JSONMap{
                "recipient_email": "admin@example.com",
            },
            wantErr: false,
        },
        {
            name: "valid full configuration",
            config: models.JSONMap{
                "recipient_email": "admin@example.com",
                "smtp_host":       "smtp.gmail.com",
                "smtp_port":       float64(587),
                "from_email":      "store@example.com",
                "smtp_username":   "store@example.com",
                "smtp_password":   "password",
            },
            wantErr: false,
        },
        {
            name:    "missing recipient_email",
            config:  models.JSONMap{},
            wantErr: true,
        },
        {
            name: "invalid recipient_email",
            config: models.JSONMap{
                "recipient_email": "not-an-email",
            },
            wantErr: true,
        },
        {
            name: "invalid smtp_port",
            config: models.JSONMap{
                "recipient_email": "admin@example.com",
                "smtp_port":       float64(99999),
            },
            wantErr: true,
        },
        {
            name: "smtp_username without password",
            config: models.JSONMap{
                "recipient_email": "admin@example.com",
                "smtp_username":   "user",
            },
            wantErr: true,
        },
        {
            name: "smtp_password without username",
            config: models.JSONMap{
                "recipient_email": "admin@example.com",
                "smtp_password":   "pass",
            },
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := handler.Validate(tt.config)
            if (err != nil) != tt.wantErr {
                t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}

func TestEmailNotificationHandler_Metadata(t *testing.T) {
    handler := NewEmailNotificationHandler()
    metadata := handler.Metadata()

    if metadata.Type != "email_notification" {
        t.Errorf("Expected type 'email_notification', got '%s'", metadata.Type)
    }

    if metadata.Name == "" {
        t.Error("Expected non-empty name")
    }

    if metadata.Description == "" {
        t.Error("Expected non-empty description")
    }

    if metadata.ConfigSchema == nil {
        t.Error("Expected non-nil config schema")
    }

    if metadata.ExampleConfig == nil {
        t.Error("Expected non-nil example config")
    }
}

func TestEmailNotificationHandler_Handle(t *testing.T) {
    handler := NewEmailNotificationHandler()

    // Note: This test doesn't actually send emails in production
    // In a real test environment, you would:
    // 1. Mock the SMTP server
    // 2. Use a test SMTP server like MailHog or MailCatcher
    // 3. Test against a sandbox email API

    payment := &models.Payment{
        ID:       "pay_test123",
        ItemID:   "item_test456",
        Status:   "confirmed",
        Amount:   "0.001",
        Currency: "BTC",
        PayerInfo: models.JSONMap{
            "email": "buyer@example.com",
        },
    }

    item := &models.Item{
        ID:   "item_test456",
        Name: "Test Product",
        BackendConfig: models.JSONMap{
            "recipient_email": "admin@example.com",
            "smtp_host":       "localhost",
            "smtp_port":       float64(1025), // MailHog default port
            "from_email":      "store@example.com",
        },
    }

    ctx := context.Background()

    // This will fail without a real SMTP server, but tests the code path
    result, err := handler.Handle(ctx, payment, item)
    
    // In CI without SMTP, we expect an error
    // In a test environment with MailHog, we would check result
    if err == nil {
        // Success case - validate result structure
        if result["email_sent"] != true {
            t.Error("Expected email_sent to be true")
        }

        if result["recipient"] != "admin@example.com" {
            t.Errorf("Expected recipient 'admin@example.com', got '%v'", result["recipient"])
        }

        if _, ok := result["sent_at"].(string); !ok {
            t.Error("Expected sent_at to be a string")
        }
    }
}

func TestIsValidEmail(t *testing.T) {
    tests := []struct {
        email string
        valid bool
    }{
        {"user@example.com", true},
        {"admin@store.io", true},
        {"test+tag@domain.co.uk", true},
        {"", false},
        {"@", false},
        {"@example.com", false},
        {"user@", false},
        {"userexample.com", false},
        {"user@@example.com", false},
        {"u", false},
    }

    for _, tt := range tests {
        t.Run(tt.email, func(t *testing.T) {
            result := isValidEmail(tt.email)
            if result != tt.valid {
                t.Errorf("isValidEmail(%q) = %v, want %v", tt.email, result, tt.valid)
            }
        })
    }
}
```

## Backend Configuration

Each item's `backend_config` is a JSON object stored in the database. The handler receives this configuration and uses it to determine behavior.

### Configuration Design Principles

1. **Required vs Optional**: Clearly distinguish required fields from optional ones
2. **Sensible Defaults**: Provide defaults for optional fields
3. **Validation**: Validate all fields in `Validate()` method
4. **Documentation**: Document each field in `Metadata()`
5. **Flexibility**: Allow configuration to evolve without breaking existing items

### Example Configuration for Email Handler

```json
{
  "recipient_email": "admin@example.com",
  "smtp_host": "smtp.gmail.com",
  "smtp_port": 587,
  "from_email": "store@example.com",
  "smtp_username": "store@example.com",
  "smtp_password": "app_password_here"
}
```

## Validation Best Practices

The `Validate()` method is your first line of defense against misconfiguration.

### What to Validate

✅ **Do validate:**
- Required fields are present
- Field types are correct (string, number, boolean)
- Values are within acceptable ranges
- Related fields are consistent (e.g., if A is set, B must also be set)
- Formats are correct (URLs, emails, regex patterns)

❌ **Don't validate:**
- Whether external services are reachable (do this in `Handle()`)
- Whether files exist on disk (may not exist at creation time)
- Database state (validation must be side-effect free)

### Validation Patterns

**Type Assertions with Error Messages:**
```go
recipientEmail, ok := config["recipient_email"].(string)
if !ok || recipientEmail == "" {
    return fmt.Errorf("recipient_email is required and must be a non-empty string")
}
```

**Range Validation:**
```go
if port, ok := config["port"].(float64); ok {
    if port < 1 || port > 65535 {
        return fmt.Errorf("port must be between 1 and 65535")
    }
}
```

**Conditional Requirements:**
```go
hasUsername := config["username"] != nil
hasPassword := config["password"] != nil

if hasUsername != hasPassword {
    return fmt.Errorf("username and password must both be provided or both omitted")
}
```

**Format Validation:**
```go
if url, ok := config["webhook_url"].(string); ok {
    if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
        return fmt.Errorf("webhook_url must start with http:// or https://")
    }
}
```

## Testing Your Handler

### Unit Tests

Test each method independently:

```go
func TestYourHandler_Validate(t *testing.T) {
    handler := NewYourHandler()
    
    tests := []struct {
        name    string
        config  models.JSONMap
        wantErr bool
    }{
        // Valid configurations
        {"valid minimal", models.JSONMap{...}, false},
        {"valid full", models.JSONMap{...}, false},
        
        // Invalid configurations
        {"missing required field", models.JSONMap{}, true},
        {"invalid type", models.JSONMap{"field": 123}, true},
        {"out of range", models.JSONMap{"port": 99999}, true},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := handler.Validate(tt.config)
            if (err != nil) != tt.wantErr {
                t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

### Integration Tests

Test the full workflow with a real database:

```go
func TestYourHandler_Integration(t *testing.T) {
    // Setup temporary BoltDB database
    tmpDir := t.TempDir()
    dbPath := filepath.Join(tmpDir, "test.db")
    
    db, err := bolt.Open(dbPath, 0600, nil)
    if err != nil {
        t.Fatalf("Failed to open database: %v", err)
    }
    defer db.Close()

    // Initialize buckets
    if err := dbpkg.InitBuckets(db); err != nil {
        t.Fatalf("Failed to initialize buckets: %v", err)
    }

    // Create registry and register handler
    registry := handler.NewRegistry()
    registry.Register(NewYourHandler())

    // Create store service
    store := store.NewStore(db, registry)

    // Create test item
    item := &models.Item{
        Name:        "Test Item",
        BackendType: "your_handler_type",
        BackendConfig: models.JSONMap{
            // Valid configuration
        },
    }
    
    // Test workflow: Create item → Payment → Fulfillment
    // ...
}
```

### Test Coverage Goals

- **Validate()**: 100% coverage - test all validation paths
- **Handle()**: >80% coverage - test success and major error cases
- **Metadata()**: 100% coverage - simple getter method

## Registering Your Handler

### Step 1: Import in main.go

```go
import (
    "github.com/opd-ai/store/internal/handlers"
    // ... other imports
)
```

### Step 2: Add to registerHandlers()

In `cmd/store/main.go`, add your handler to the registration list:

```go
func registerHandlers(registry *handler.Registry) error {
    handlersToRegister := []handler.FulfillmentHandler{
        handlers.NewDigitalMediaHandler(),
        handlers.NewShippingFormHandler(),
        handlers.NewPrintOnDemandHandler(),
        handlers.NewCustomHandler(),
        handlers.NewEmailNotificationHandler(), // Your new handler
    }

    for _, h := range handlersToRegister {
        if err := registry.Register(h); err != nil {
            return fmt.Errorf("failed to register handler: %w", err)
        }
    }

    return nil
}
```

### Step 3: Restart the Server

```bash
go run cmd/store/main.go
```

### Step 4: Verify Registration

```bash
curl -H "X-Admin-Token: your-token" http://localhost:8080/admin/handlers
```

You should see your handler in the list with its metadata.

## Advanced Topics

### Idempotency

The `Handle()` method may be called multiple times for the same payment (e.g., if the server crashes mid-fulfillment). Design your handler to be idempotent:

```go
func (h *YourHandler) Handle(ctx context.Context, payment *models.Payment, item *models.Item) (models.JSONMap, error) {
    // Check if already fulfilled
    if existingResult, ok := payment.FulfillmentResult["order_id"]; ok {
        // Already processed, return existing result
        return payment.FulfillmentResult, nil
    }
    
    // Proceed with fulfillment...
}
```

### Error Handling and Retries

Return errors for transient failures. The store service may retry later:

```go
resp, err := http.Post(webhookURL, "application/json", body)
if err != nil {
    return nil, fmt.Errorf("failed to call webhook: %w", err)
}

if resp.StatusCode >= 500 {
    return nil, fmt.Errorf("webhook returned server error: %d", resp.StatusCode)
}
```

### Long-Running Operations

For operations that take >5 seconds, consider async processing:

```go
func (h *YourHandler) Handle(ctx context.Context, payment *models.Payment, item *models.Item) (models.JSONMap, error) {
    // Initiate async operation
    jobID := startAsyncJob(payment, item)
    
    // Return immediately with job ID
    return models.JSONMap{
        "status": "processing",
        "job_id": jobID,
    }, nil
}
```

Provide a separate endpoint to check job status.

### Accessing the Database

If your handler needs database access, accept it in the constructor:

```go
type YourHandler struct {
    db *bolt.DB
}

func NewYourHandler(db *bolt.DB) *YourHandler {
    return &YourHandler{db: db}
}
```

Update `cmd/store/main.go` to pass the database:

```go
handlers.NewYourHandler(db),
```

### Logging

Use structured logging for debugging:

```go
func (h *YourHandler) Handle(ctx context.Context, payment *models.Payment, item *models.Item) (models.JSONMap, error) {
    log.Printf("[YourHandler] Processing payment %s for item %s", payment.ID, item.ID)
    
    // ...
    
    log.Printf("[YourHandler] Successfully fulfilled payment %s", payment.ID)
    return result, nil
}
```

## Summary

Creating a custom handler involves:

1. **Implement the interface**: `Handle()`, `Validate()`, `Metadata()`
2. **Design configuration**: Define required and optional fields
3. **Validate thoroughly**: Check all configuration in `Validate()`
4. **Test comprehensively**: Unit tests for validation, integration tests for fulfillment
5. **Register**: Add to `registerHandlers()` in main.go
6. **Document**: Provide clear metadata and examples

Your handler is now ready to use! Create items with `"backend_type": "your_handler_type"` and they'll be fulfilled by your custom logic.

## Next Steps

- Review the [built-in handlers](../../internal/handlers/) for more examples
- Read the [API documentation](../api/README.md) to understand the full workflow
- Check the [deployment guide](../deployment/README.md) for production considerations
- Explore [troubleshooting](../troubleshooting/README.md) if you encounter issues
