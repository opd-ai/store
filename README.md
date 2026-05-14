# opd-ai/store

[![CI](https://github.com/opd-ai/store/workflows/CI/badge.svg)](https://github.com/opd-ai/store/actions)
[![Coverage](https://codecov.io/gh/opd-ai/store/branch/main/graph/badge.svg)](https://codecov.io/gh/opd-ai/store)
[![Go Report Card](https://goreportcard.com/badge/github.com/opd-ai/store)](https://goreportcard.com/report/github.com/opd-ai/store)

A self-hosted cryptocurrency payment store with pluggable fulfillment handlers. Integrates with **opd-ai/paywall** for Bitcoin/Monero payments and provides a flexible, extensible system for selling digital goods, physical products, print-on-demand items, or anything custom.

## Features

- **Pluggable Fulfillment Handlers** - Digital downloads, shipping forms, print-on-demand, or custom webhooks
- **Cryptocurrency Payments** - Bitcoin and Monero via opd-ai/paywall integration
- **Admin API** - Manage items, categories, and tags without code changes
- **BoltDB Storage** - Embedded key-value database with full CRUD operations
- **RESTful API** - CORS-enabled endpoints for frontends and integrations
- **Docker Ready** - Complete development environment with Docker Compose
- **Handler Registry** - Dynamically register and dispatch fulfillment strategies
- **JSON Configuration** - Flexible backend configuration stored in database

## Requirements

- Go 1.21+
- Docker & Docker Compose (optional, for local development)

## Quick Start

### Docker Compose (Recommended)

```bash
git clone https://github.com/opd-ai/store.git
cd store
docker-compose up -d

# Verify running
curl http://localhost:8080/health
```

Services available at:
- API: http://localhost:8080
- Mock Paywall: http://localhost:8081

### Local Setup

```bash
go mod download
cp .env.example .env
# Edit .env with your database URL
go run cmd/store/main.go
```

## Configuration

Set environment variables in `.env`:

```bash
STORE_DATABASE_PATH=./data/store.db
STORE_PORT=8080
STORE_HOST=0.0.0.0
STORE_PAYWALL_URL=http://localhost:8081
STORE_PAYWALL_API_KEY=sk_test_12345
STORE_PAYWALL_WEBHOOK_SECRET=webhook_secret_12345
STORE_PUBLIC_URL=http://localhost:8080
STORE_AUTO_FULFILL=true
STORE_ADMIN_TOKEN=your-secret-token
STORE_LOG_LEVEL=debug
STORE_LOG_FORMAT=json
```

### Configuration Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `STORE_DATABASE_PATH` | No | Path to BoltDB database file (default: ./data/store.db) |
| `STORE_PORT` | No | Server port (default: 8080) |
| `STORE_HOST` | No | Server host (default: 0.0.0.0) |
| `STORE_PAYWALL_URL` | Yes | URL of the opd-ai/paywall service |
| `STORE_PAYWALL_API_KEY` | Yes | API key for paywall authentication |
| `STORE_PAYWALL_WEBHOOK_SECRET` | Yes | Secret for verifying webhook signatures |
| `STORE_PUBLIC_URL` | Yes | Public URL of this store (for webhook callbacks) |
| `STORE_AUTO_FULFILL` | No | Auto-fulfill payments after confirmation (default: true) |
| `STORE_ADMIN_TOKEN` | Yes | Token for admin API authentication |
| `STORE_LOG_LEVEL` | No | Log level: debug, info, warn, error (default: info) |
| `STORE_LOG_FORMAT` | No | Log format: json or text (default: json) |

## Usage Examples

### Create Category
```bash
curl -X POST http://localhost:8080/admin/categories \
  -H "Content-Type: application/json" \
  -H "X-Admin-Token: your-secret-token" \
  -d '{"name": "Digital Products", "description": "E-books and digital downloads"}'
```

**Response:**
```json
{
  "id": "cat_abc123",
  "name": "Digital Products",
  "description": "E-books and digital downloads",
  "order": 0,
  "created_at": "2026-05-13T16:00:00Z"
}
```

### Create Item
```bash
curl -X POST http://localhost:8080/admin/items \
  -H "Content-Type: application/json" \
  -H "X-Admin-Token: your-secret-token" \
  -d '{
    "name": "E-Book: Go Programming",
    "description": "Comprehensive guide to Go",
    "price": "0.001",
    "currency": "BTC",
    "category_id": "cat_abc123",
    "backend_type": "digital_media",
    "backend_config": {
      "storage": "local",
      "file_path": "./downloads/go-programming.pdf",
      "expiration_hours": 24
    }
  }'
```

**Response:**
```json
{
  "id": "item_xyz789",
  "name": "E-Book: Go Programming",
  "description": "Comprehensive guide to Go",
  "price": "0.001",
  "currency": "BTC",
  "category_id": "cat_abc123",
  "backend_type": "digital_media",
  "backend_config": {
    "storage": "local",
    "file_path": "./downloads/go-programming.pdf",
    "expiration_hours": 24
  },
  "active": true,
  "created_at": "2026-05-13T16:05:00Z"
}
```

### View Catalog
```bash
curl http://localhost:8080/api/catalog
```

**Response:**
```json
{
  "categories": [
    {
      "id": "cat_abc123",
      "name": "Digital Products",
      "description": "E-books and digital downloads"
    }
  ],
  "items": [
    {
      "id": "item_xyz789",
      "name": "E-Book: Go Programming",
      "price": "0.001",
      "currency": "BTC",
      "category_id": "cat_abc123",
      "backend_type": "digital_media"
    }
  ]
}
```

### Initiate Checkout
```bash
curl -X POST http://localhost:8080/api/checkout \
  -H "Content-Type: application/json" \
  -d '{"item_id": "item_xyz789", "email": "buyer@example.com"}'
```

**Response:**
```json
{
  "payment_id": "pay_def456",
  "invoice_id": "inv_ghi789",
  "status": "pending",
  "amount": "0.001",
  "currency": "BTC",
  "payment_address": "bc1qxy2kgdygjrsqtzq2n0yrf2493p83kkfjhx0wlh",
  "qr_code": "data:image/png;base64,...",
  "expires_at": "2026-05-13T16:35:00Z"
}
```

### Check Payment Status
```bash
curl http://localhost:8080/api/payment/pay_def456/status
```

**Response:**
```json
{
  "status": "pending",
  "amount": "0.001",
  "currency": "BTC",
  "created_at": "2026-05-13T16:05:00Z",
  "payment_address": "bc1qxy2kgdygjrsqtzq2n0yrf2493p83kkfjhx0wlh"
}
```

After payment is confirmed, status becomes `"confirmed"`, and after fulfillment:
```json
{
  "status": "fulfilled",
  "amount": "0.001",
  "currency": "BTC",
  "fulfillment_result": {
    "download_url": "http://localhost:8080/download/abc123token",
    "expires_at": "2026-05-14T16:05:00Z"
  },
  "fulfilled_at": "2026-05-13T16:10:00Z"
}
```

## Fulfillment Handlers

opd-ai/store includes four handler types, configured per-item via `backend_type` and `backend_config`. Each handler implements the `FulfillmentHandler` interface with three methods: `Handle()` for fulfillment logic, `Validate()` for configuration validation, and `Metadata()` for handler documentation.

| Handler | Purpose |
|---------|---------|
| **digital_media** | Immediate downloads with expiration and rate limiting |
| **shipping_form** | Collect customer address for physical goods fulfillment |
| **pod** | Print-on-demand integration (Printful) |
| **custom** | External API webhooks for custom fulfillment workflows |

## Creating Custom Handlers

1. Create handler in `internal/handlers/`:

```go
package handlers

import (
    "context"
    "github.com/opd-ai/store/pkg/handler"
    "github.com/opd-ai/store/pkg/models"
)

type MyHandler struct{}

func (h *MyHandler) Handle(ctx context.Context, payment *models.Payment, item *models.Item) (map[string]interface{}, error) {
    return map[string]interface{}{"status": "processed"}, nil
}

func (h *MyHandler) Validate(config models.JSONMap) error {
    // Validate handler configuration
    return nil
}

func (h *MyHandler) Metadata() handler.HandlerMetadata {
    return handler.HandlerMetadata{
        Type: "my_handler",
        Description: "My custom fulfillment handler",
    }
}
```

2. Register in `cmd/store/main.go`:
```go
registry.Register(handlers.NewMyHandler())
```

## Data Models

The system provides core models for managing catalog and payments:

- **Category** - Groups items with name, slug, and metadata
- **Tag** - Tags for item classification with many-to-many relationships
- **Item** - Product listing with price, currency, backend type, and configuration
- **Payment** - Records transactions with status tracking (pending, confirmed, fulfilled, failed)
- **FormSubmission** - Stores form data from shipping and custom handlers

All models use JSON encoding for BoltDB storage with automatic bucket initialization on startup.

## Testing

```bash
# Run all tests
go test ./... -v

# Run with coverage
go test -v -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## License

MIT - See [LICENSE](LICENSE)

## Documentation

- [DESIGN.md](DESIGN.md) - Comprehensive architecture and design specifications
- [ARCHITECTURE.md](ARCHITECTURE.md) - System diagrams and component flows
- [CONTRIBUTING.md](CONTRIBUTING.md) - Contributing guidelines and development setup
