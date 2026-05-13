# Handlers Documentation

This directory contains documentation for fulfillment handlers in opd-ai/store.

## Contents

- **[DEVELOPMENT_GUIDE.md](./DEVELOPMENT_GUIDE.md)** - Complete guide to creating custom handlers
  - Understanding the FulfillmentHandler interface
  - Step-by-step tutorial with EmailNotificationHandler example
  - Validation best practices
  - Testing strategies
  - Registration and deployment

## Quick Start

Want to create a custom handler? Start with the [Development Guide](./DEVELOPMENT_GUIDE.md).

## Built-in Handlers

opd-ai/store includes four handlers out of the box:

### 1. Digital Media Handler (`digital_media`)
Delivers digital downloads with expiration and rate limiting.

**Configuration:**
```json
{
  "storage": "s3",
  "s3_bucket": "my-downloads",
  "s3_region": "us-east-1",
  "s3_key": "products/ebook.pdf",
  "expiration_hours": 24,
  "max_downloads": 3
}
```

**Use cases:**
- E-books
- Software licenses
- Digital art
- Music/video files

### 2. Shipping Form Handler (`shipping_form`)
Collects shipping address from customer for physical goods.

**Configuration:**
```json
{
  "required_fields": ["name", "address", "city", "zip", "country"]
}
```

**Use cases:**
- Physical products
- Print materials
- Merchandise
- Any item requiring shipping

### 3. Print-on-Demand Handler (`pod`)
Integrates with print-on-demand providers (Printful, Redbubble, TeeSpring).

**Configuration:**
```json
{
  "provider": "printful",
  "product_id": "71",
  "variant_id": "4012",
  "printful_api_key": "your-api-key"
}
```

**Use cases:**
- T-shirts
- Posters
- Mugs
- Custom printed products

### 4. Custom Webhook Handler (`custom`)
Calls external webhook for custom fulfillment logic.

**Configuration:**
```json
{
  "webhook_url": "https://example.com/fulfill",
  "method": "POST",
  "headers": {
    "Authorization": "Bearer token"
  },
  "retry_count": 3,
  "timeout_seconds": 30
}
```

**Use cases:**
- Integration with existing systems
- Complex multi-step workflows
- Third-party services
- Custom business logic

## Handler Lifecycle

```
1. Item Created → Validate(backend_config)
   ↓
   validates configuration
   ↓
2. Payment Confirmed → Handle(payment, item)
   ↓
   performs fulfillment
   ↓
3. Returns fulfillment_result
   ↓
   stored in payment record
```

## Registry

Handlers are registered at startup in `cmd/store/main.go`:

```go
func registerHandlers(registry *handler.Registry) error {
    handlersToRegister := []handler.FulfillmentHandler{
        handlers.NewDigitalMediaHandler(),
        handlers.NewShippingFormHandler(),
        handlers.NewPrintOnDemandHandler(),
        handlers.NewCustomHandler(),
    }

    for _, h := range handlersToRegister {
        if err := registry.Register(h); err != nil {
            return fmt.Errorf("failed to register handler: %w", err)
        }
    }

    return nil
}
```

To see all registered handlers, call:

```bash
curl -H "X-Admin-Token: your-token" http://localhost:8080/admin/handlers
```

## Creating Items with Handlers

When creating an item, specify the `backend_type` matching a registered handler:

```bash
curl -X POST http://localhost:8080/admin/items \
  -H "Content-Type: application/json" \
  -H "X-Admin-Token: your-token" \
  -d '{
    "name": "My Product",
    "price": "0.001",
    "currency": "BTC",
    "backend_type": "digital_media",
    "backend_config": {
      "storage": "local",
      "file_path": "./downloads/product.pdf",
      "expiration_hours": 24
    }
  }'
```

The `backend_config` will be validated by the handler's `Validate()` method before the item is created.

## Resources

- [Full Development Guide](./DEVELOPMENT_GUIDE.md)
- [API Documentation](../api/README.md)
- [Built-in Handler Source Code](../../internal/handlers/)
- [Handler Interface Definition](../../pkg/handler/handler.go)

## Community Handlers

Have you built a handler you'd like to share? Open a PR to add it to this list!

*No community handlers yet - be the first to contribute!*
