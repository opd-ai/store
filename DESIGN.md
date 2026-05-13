# opd-ai/store: Architecture Design Document

**Status**: Design Phase  
**Version**: 1.0  
**Date**: May 2026

## Executive Summary

opd-ai/store is a self-hosted, pluggable cryptocurrency store system that integrates opd-ai/paywall for payment authorization and delegates fulfillment to swappable backend handlers. The system decouples payment verification from fulfillment logic, enabling support for diverse use cases: digital downloads, physical goods with address collection, print-on-demand services, and custom workflows—all via a unified admin interface.

---

## 1. System Architecture

### 1.1 High-Level Flow

```
User Request
    ↓
[Store Frontend] ← Item Catalog, Tags, Categories
    ↓
[Payment Gateway] ← opd-ai/paywall (Bitcoin/Monero verification)
    ↓
Payment Confirmed
    ↓
[FulfillmentHandler] ← Dynamic dispatch based on item.BackendType
    ├─ DigitalMedia Handler
    ├─ ShippingForm Handler
    ├─ PrintOnDemand Handler
    └─ Custom Handler
    ↓
[Fulfillment Output] ← Backend-specific action
```

### 1.2 Component Responsibilities

| Component | Responsibility |
|-----------|-----------------|
| **Frontend** | Display catalog, collect user selections, integrate QR/payment UI |
| **Store API** | Manage items/categories/backends, serve catalog, trigger fulfillment |
| **Paywall Integration** | Verify payment signatures, confirm blockchain transactions |
| **FulfillmentHandler** | Execute backend-specific post-payment logic |
| **Admin Panel** | CRUD operations for items, categories, handler configuration |
| **Storage** | Persist configuration, metadata; optional state/transaction logs |

---

## 2. Data Models

### 2.1 Core Entities

#### Category
```
ID:         UUID
Name:       string
Slug:       string (unique, URL-safe)
Description: string
Order:      int (for display ordering)
Metadata:   map[string]string (for custom attributes)
CreatedAt:  timestamp
UpdatedAt:  timestamp
```

#### Tag
```
ID:         UUID
Name:       string
Slug:       string (unique)
CreatedAt:  timestamp
```

#### Item
```
ID:         UUID
CategoryID: UUID (foreign key)
Name:       string
Description: string
Price:      decimal (in satoshis or monero atomic units, or base currency with conversion)
Currency:   string (e.g., "BTC", "XMR", "USD")
Image:      string (URL or local path)
Tags:       []Tag
BackendType: string (enum: "digital_media", "shipping_form", "pod", "custom")
BackendConfig: map[string]interface{} (handler-specific configuration)
Metadata:   map[string]string
Active:     bool
CreatedAt:  timestamp
UpdatedAt:  timestamp
```

#### Payment
```
ID:         UUID
ItemID:     UUID (foreign key)
PaymentHash: string (transaction ID from paywall)
Status:     string (enum: "pending", "confirmed", "failed", "fulfilled")
PayerInfo:  map[string]string (email, anon ID, custom fields)
Amount:     decimal
Currency:   string
ConfirmedAt: timestamp (nil if not yet confirmed)
FulfilledAt: timestamp (nil if not yet fulfilled)
FulfillmentResult: map[string]interface{} (handler-specific output)
CreatedAt:  timestamp
UpdatedAt:  timestamp
```

#### BackendConfig (persisted as JSON in Item)
```
Type:       string (handler type identifier)
Settings:   map[string]interface{} (handler-specific settings)
Params:     map[string]interface{} (handler-specific request params)
Secrets:    map[string]string (API keys, tokens—encrypted at rest)
```

---

## 3. FulfillmentHandler Interface

### 3.1 Interface Definition

```go
// FulfillmentHandler defines the contract for post-payment fulfillment logic.
type FulfillmentHandler interface {
    /// Handle processes the payment and executes backend-specific fulfillment.
    //
    // Parameters:
    // - ctx: context for cancellation and timeouts
    // - payment: Payment object with verified payment details
    // - itemConfig: Item metadata with backend-specific configuration
    //
    // Returns:
    // - result: map[string]interface{} with backend-specific output
    //   Common keys: "download_url", "form_url", "status", "tracking_id"
    // - error: nil on success; backend-specific error on failure
    Handle(ctx context.Context, payment *Payment, itemConfig *Item) (map[string]interface{}, error)

    /// Validate checks configuration validity before item creation/update.
    //
    // Returns:
    // - error: nil if config is valid; descriptive error otherwise
    Validate(config map[string]interface{}) error

    /// Metadata returns handler information for admin UI and discovery.
    //
    // Returns:
    // - HandlerMetadata with name, description, required fields
    Metadata() HandlerMetadata
}

// HandlerMetadata describes a FulfillmentHandler for discovery and configuration.
type HandlerMetadata struct {
    Type:           string   // e.g., "digital_media"
    DisplayName:    string   // e.g., "Digital Media Download"
    Description:    string
    RequiredFields: []Field  // Configuration fields required for this handler
    OptionalFields: []Field
}

type Field struct {
    Name:        string
    Type:        string   // "string", "number", "boolean", "secret"
    Description: string
    Example:     string
    Validation:  string   // regex or validation rule
}
```

### 3.2 Handler Registry

```go
// Registry manages registered FulfillmentHandlers.
type HandlerRegistry struct {
    handlers map[string]FulfillmentHandler
    mu       sync.RWMutex
}

func (r *HandlerRegistry) Register(handler FulfillmentHandler) error
func (r *HandlerRegistry) Get(handlerType string) (FulfillmentHandler, error)
func (r *HandlerRegistry) All() map[string]HandlerMetadata
```

---

## 4. Backend Implementations

### 4.1 DigitalMedia Handler

**Purpose**: Serve downloadable files (ebooks, software, assets) immediately after payment confirmation.

**Use Case**: User A purchases a digital product, receives immediate download link or S3 redirect.

**Configuration**:
```json
{
  "type": "digital_media",
  "settings": {
    "storage": "s3",  // or "local"
    "s3_bucket": "store-downloads",
    "s3_region": "us-east-1",
    "s3_key_prefix": "items/",
    "expiration_hours": 24,
    "max_downloads": 10
  }
}
```

**Handle() Process**:
1. Verify payment is confirmed
2. Retrieve file path/S3 key from itemConfig
3. Generate pre-signed S3 URL (if S3) or serve file directly
4. Return `{"download_url": "...", "expires_at": "..."}`

**Output**:
```json
{
  "download_url": "https://s3.amazonaws.com/store-downloads/items/...",
  "expires_at": "2026-05-13T14:30:00Z",
  "file_size_mb": 25
}
```

---

### 4.2 ShippingForm Handler

**Purpose**: Collect shipping address after payment, store submission for fulfillment review.

**Use Case**: User B purchases a physical item, enters shipping address, admin reviews and ships manually.

**Configuration**:
```json
{
  "type": "shipping_form",
  "settings": {
    "form_fields": {
      "address1": {"label": "Street Address", "required": true},
      "address2": {"label": "Apt/Suite (Optional)", "required": false},
      "city": {"label": "City", "required": true},
      "state": {"label": "State/Province", "required": true},
      "postal_code": {"label": "Postal Code", "required": true},
      "country": {"label": "Country", "required": true}
    },
    "require_phone": true,
    "require_notes": false
  }
}
```

**Handle() Process**:
1. Generate form URL with embedded payment ID
2. Store form submission (address, phone, notes)
3. Mark payment as "pending_fulfillment"
4. Return form URL and confirmation details

**Output**:
```json
{
  "form_url": "https://store.example.com/fulfill/address/payment-id-123",
  "status": "awaiting_address",
  "timeout_minutes": 60
}
```

**Form Submission**:
```json
{
  "address1": "123 Main St",
  "address2": "Apt 4B",
  "city": "Springfield",
  "state": "IL",
  "postal_code": "62701",
  "country": "US",
  "phone": "+1-217-555-1234",
  "notes": "Please leave at front desk"
}
```

---

### 4.3 PrintOnDemand Handler

**Purpose**: Delegate to external PoD service (Printful, Redbubble, etc.) and return order tracking.

**Use Case**: User C purchases a custom printed item; system creates order with PoD vendor and returns tracking.

**Configuration**:
```json
{
  "type": "pod",
  "settings": {
    "provider": "printful",
    "api_key": "<encrypted>",
    "api_url": "https://api.printful.com",
    "product_mapping": {
      "item-id-123": {
        "product_id": 456,
        "variant_id": 789,
        "size": "L",
        "color": "black"
      }
    },
    "webhook_secret": "<encrypted>"
  }
}
```

**Handle() Process**:
1. Verify payment confirmation
2. Retrieve product mapping from config
3. Call Printful API with customer address (if collected) and product details
4. Return order ID and tracking URL

**Output**:
```json
{
  "provider": "printful",
  "order_id": "5678",
  "tracking_url": "https://printful.com/track/5678",
  "status": "processing",
  "estimated_ship_date": "2026-05-20"
}
```

---

### 4.4 Custom Handler

**Purpose**: Execute arbitrary fulfillment logic via webhook or embedded script.

**Use Case**: User D integrates a custom API, serverless function, or internal system.

**Configuration**:
```json
{
  "type": "custom",
  "settings": {
    "webhook_url": "https://internal.example.com/fulfill",
    "webhook_method": "POST",
    "webhook_headers": {
      "Authorization": "Bearer <token>"
    },
    "timeout_seconds": 30,
    "retry_count": 3,
    "payload_template": {
      "item_id": "{item_id}",
      "payment_hash": "{payment_hash}",
      "payer_email": "{payer_email}",
      "custom_field": "value"
    }
  }
}
```

**Handle() Process**:
1. Build payload from template and payment data
2. POST to webhook URL with retries
3. Parse JSON response and return result

**Input (sent to webhook)**:
```json
{
  "item_id": "item-123",
  "payment_hash": "abc123def456",
  "payer_email": "user@example.com",
  "custom_field": "value"
}
```

**Output (expected from webhook)**:
```json
{
  "status": "success",
  "fulfillment_id": "custom-001",
  "custom_data": {}
}
```

---

## 5. Configuration & Storage

### 5.1 Configuration Sources (Priority Order)

1. **Environment Variables** (highest priority)
   - `STORE_DATABASE_URL`: PostgreSQL connection string
   - `STORE_PAYWALL_URL`: opd-ai/paywall service URL
   - `STORE_PORT`: Server port (default: 8080)
   - `STORE_ADMIN_TOKEN`: API token for admin endpoints
   - `STORE_TEMPLATES_DIR`: Path to custom HTML/CSS templates
   - Handler-specific: `STORE_HANDLER_DIGITAL_MEDIA_S3_BUCKET`, etc.

2. **Config File** (config.yaml in app root)
   ```yaml
   server:
     port: 8080
     host: 0.0.0.0
     tls_cert: /etc/tls/cert.pem
     tls_key: /etc/tls/key.pem
   
   paywall:
     url: https://paywall.example.com
     api_key: <encrypted>
   
   database:
     type: postgresql
     url: postgres://user:pass@localhost/store_db
   
   storage:
     type: local  # or "s3"
     local_path: ./data
     s3_bucket: store-data
     s3_region: us-east-1
   
   handlers:
     digital_media:
       enabled: true
       storage: s3
     shipping_form:
       enabled: true
     pod:
       enabled: true
       provider: printful
     custom:
       enabled: true
   ```

3. **Defaults** (lowest priority)

### 5.2 Database Schema

**PostgreSQL** (or compatible)

```sql
CREATE TABLE categories (
  id UUID PRIMARY KEY,
  name VARCHAR(255) NOT NULL,
  slug VARCHAR(255) UNIQUE NOT NULL,
  description TEXT,
  `order` INT DEFAULT 0,
  metadata JSONB DEFAULT '{}',
  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE tags (
  id UUID PRIMARY KEY,
  name VARCHAR(255) NOT NULL,
  slug VARCHAR(255) UNIQUE NOT NULL,
  created_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE items (
  id UUID PRIMARY KEY,
  category_id UUID NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
  name VARCHAR(255) NOT NULL,
  description TEXT,
  price DECIMAL(20, 8) NOT NULL,
  currency VARCHAR(10) NOT NULL,
  image VARCHAR(500),
  backend_type VARCHAR(50) NOT NULL,
  backend_config JSONB NOT NULL DEFAULT '{}',
  metadata JSONB DEFAULT '{}',
  active BOOLEAN DEFAULT true,
  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW(),
  INDEX(category_id),
  INDEX(backend_type),
  INDEX(active)
);

CREATE TABLE item_tags (
  item_id UUID NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  tag_id UUID NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
  PRIMARY KEY (item_id, tag_id)
);

CREATE TABLE payments (
  id UUID PRIMARY KEY,
  item_id UUID NOT NULL REFERENCES items(id),
  payment_hash VARCHAR(255) UNIQUE NOT NULL,
  status VARCHAR(50) NOT NULL,
  payer_info JSONB DEFAULT '{}',
  amount DECIMAL(20, 8) NOT NULL,
  currency VARCHAR(10) NOT NULL,
  confirmed_at TIMESTAMP,
  fulfilled_at TIMESTAMP,
  fulfillment_result JSONB,
  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW(),
  INDEX(item_id),
  INDEX(payment_hash),
  INDEX(status),
  INDEX(created_at)
);

CREATE TABLE form_submissions (
  id UUID PRIMARY KEY,
  payment_id UUID NOT NULL REFERENCES payments(id) ON DELETE CASCADE,
  form_data JSONB NOT NULL,
  submitted_at TIMESTAMP DEFAULT NOW(),
  processed BOOLEAN DEFAULT false,
  created_at TIMESTAMP DEFAULT NOW()
);
```

---

## 6. API Contract

### 6.1 Store API (Public)

#### GET /api/catalog
Returns catalog with categories, items, and tags.
```json
{
  "categories": [
    {
      "id": "...",
      "name": "E-Books",
      "items": [...]
    }
  ],
  "tags": [...],
  "timestamp": "2026-05-12T14:00:00Z"
}
```

#### GET /api/items/:id
Returns item details with metadata.
```json
{
  "id": "...",
  "name": "Mastering Go",
  "price": 9999,
  "currency": "BTC",
  "backend_type": "digital_media",
  "metadata": {...}
}
```

#### POST /api/checkout
Initiates payment flow.
```json
{
  "item_id": "...",
  "payer_email": "user@example.com"
}
```
Response:
```json
{
  "payment_id": "...",
  "paywall_url": "https://paywall.example.com/invoice/...",
  "status": "pending"
}
```

#### GET /api/payment/:id/status
Polls payment status.
```json
{
  "id": "...",
  "status": "confirmed",
  "fulfillment": {
    "type": "digital_media",
    "download_url": "...",
    "expires_at": "..."
  }
}
```

### 6.2 Admin API

#### POST /admin/categories
Create category (requires `X-Admin-Token` header).

#### GET/PUT/DELETE /admin/categories/:id
Manage category.

#### POST /admin/items
Create item with backend configuration.

#### GET/PUT/DELETE /admin/items/:id
Manage item.

#### GET /admin/payments
List payments with filters and status.

#### GET /admin/handlers
Get registered handler types and their configuration schemas.

#### POST /admin/test-fulfillment
Test a handler with mock data (admin feature).

---

## 7. File Storage & Templating

### 7.1 Directory Structure

```
/store-root/
├── templates/            # Custom HTML/CSS/JS (optional)
│   ├── catalog.html
│   ├── payment.html
│   ├── confirmation.html
│   └── style.css
├── config/
│   ├── config.yaml       # Main configuration
│   └── handlers/         # Handler-specific configs (optional)
├── data/
│   ├── uploads/          # User uploads (addresses, files)
│   └── logs/
└── certificates/         # TLS certs (if using HTTPS)
```

### 7.2 Template Customization

- **Tech**: Go `html/template` with custom functions
- **Customization**: Admin can upload/edit HTML/CSS without recompilation
- **Template Functions**: `i18n()`, `formatPrice()`, `qrCode()`, etc.
- **Example**:
  ```html
  <h1>{{ i18n "catalog.title" }}</h1>
  {{ range .Items }}
    <div class="item">
      <p>{{ .Name }}</p>
      <p>{{ formatPrice .Price .Currency }}</p>
    </div>
  {{ end }}
  ```

### 7.3 Localization

- **Approach**: JSON language files (en, es, fr, etc.)
  ```json
  {
    "catalog.title": "Our Store",
    "catalog.search": "Search items",
    "payment.pending": "Waiting for payment confirmation..."
  }
  ```
- **Fallback**: English for missing translations
- **Admin Feature**: Upload/edit translation files

---

## 8. Security Considerations

### 8.1 Payment Verification

- Rely on opd-ai/paywall for robust signature verification
- Store `payment_hash` (transaction ID) as unique identifier
- Never process fulfillment until paywall confirms transaction

### 8.2 Secret Management

- Handler API keys, tokens, and webhook secrets stored encrypted in database
- Decryption key managed via environment variable or external KMS
- Admin API requires bearer token (regenerate regularly)

### 8.3 Webhook Security

- Custom handler webhooks signed with HMAC-SHA256
- Include `X-Signature` header; webhook recipient must verify
- Timeout and retry limits to prevent hanging fulfillment

### 8.4 Frontend Security

- CSRF token for form submissions
- Rate limiting on checkout endpoint
- Prevent payment duplication (idempotency key)

### 8.5 Data Privacy

- Payer info (email, address) stored securely
- Audit log for admin actions
- GDPR: Support data deletion for completed transactions after 90 days

---

## 9. Deployment & Operations

### 9.1 Docker Compose (Reference)

```yaml
version: '3.8'
services:
  store:
    build: .
    ports:
      - "8080:8080"
    environment:
      STORE_DATABASE_URL: postgres://store:password@postgres:5432/store_db
      STORE_PAYWALL_URL: http://paywall:8081
      STORE_ADMIN_TOKEN: supersecret
    depends_on:
      - postgres
      - paywall

  postgres:
    image: postgres:15
    environment:
      POSTGRES_USER: store
      POSTGRES_PASSWORD: password
      POSTGRES_DB: store_db
    volumes:
      - postgres_data:/var/lib/postgresql/data

  paywall:
    image: opd-ai/paywall:latest
    ports:
      - "8081:8081"
    environment:
      PAYWALL_NETWORK: regtest  # or mainnet/testnet

volumes:
  postgres_data:
```

### 9.2 Monitoring & Logging

- Prometheus metrics: payment count, fulfillment latency, handler errors
- Structured JSON logging with context (request ID, customer ID)
- Alert on fulfillment failures or paywall connectivity issues

---

## 10. Testing Strategy

### 10.1 Unit Tests

- Handler interface: each implementation tested independently
- Validation logic for configurations
- Template rendering with various locale/currency combinations

### 10.2 Integration Tests

- Payment → fulfillment flow end-to-end
- Database transactions and rollback behavior
- Mock paywall API responses

### 10.3 Handler-Specific Tests

- **DigitalMedia**: Verify S3 URL generation, expiration
- **ShippingForm**: Form validation, data persistence
- **PoD**: Webhook payload construction, provider API calls
- **Custom**: Webhook retry logic, timeout handling

---

## 11. Future Enhancements

- Multi-currency price conversion (fiat to BTC/XMR)
- Subscription model for recurring payments
- Affiliate/referral system with custom handler
- Advanced analytics dashboard
- Webhook event system for external integrations
- Batch item import/export
- A/B testing variants of items

---

## Glossary

- **Payment Hash**: Blockchain transaction ID (from paywall verification)
- **FulfillmentHandler**: Pluggable interface for post-payment actions
- **Backend Type**: Enumeration of handler types (digital_media, shipping_form, pod, custom)
- **Payload Template**: String template with placeholders for custom handler input
- **Pre-signed URL**: Temporary S3 URL with embedded credentials (no additional auth needed)

