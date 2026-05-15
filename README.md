# opd-ai/store

[![CI](https://github.com/opd-ai/store/workflows/CI/badge.svg)](https://github.com/opd-ai/store/actions)
[![Coverage](https://codecov.io/gh/opd-ai/store/branch/main/graph/badge.svg)](https://codecov.io/gh/opd-ai/store)
[![Go Report Card](https://goreportcard.com/badge/github.com/opd-ai/store)](https://goreportcard.com/report/github.com/opd-ai/store)

A self-hosted cryptocurrency payment store with pluggable fulfillment handlers. Features **embedded paywall** integration for Bitcoin/Monero payments with optional **multisig escrow** for physical goods.

## Features

- **Embedded Paywall** - Direct integration with opd-ai/paywall library (no external server needed)
- **Multisig Escrow** - 2-of-3 escrow protection for physical goods with dispute resolution
- **Pluggable Fulfillment Handlers** - Digital downloads, shipping forms, print-on-demand, or custom webhooks
- **Cryptocurrency Payments** - Bitcoin and Monero with single-sig or multisig support
- **Per-Handler Payment Modes** - Single-sig for digital goods, escrow for physical items
- **Admin API** - Manage items, categories, and tags without code changes
- **BoltDB Storage** - Embedded key-value database with full CRUD operations
- **RESTful API** - CORS-enabled endpoints for frontends and integrations
- **Docker Ready** - Complete development environment with Docker Compose
- **Handler Registry** - Dynamically register and dispatch fulfillment strategies
- **JSON Configuration** - Flexible backend configuration stored in database

## Payment Modes

The store supports different payment modes optimized for each product type:

| Product Type | Handler | Payment Mode | Description |
|--------------|---------|--------------|-------------|
| Digital Media | `digital_media` | Single-sig | Instant delivery after confirmation |
| Physical Goods | `shipping_form` | Multisig Escrow | 2-of-3 escrow with buyer protection |
| Print-on-Demand | `pod` | Single-sig | Provider-managed fulfillment |

See [docs/ESCROW.md](docs/ESCROW.md) for detailed escrow documentation.

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

# Embedded Paywall Configuration
STORE_PAYWALL_TESTNET=true
STORE_PAYWALL_DB_PATH=./data/paywall.db
STORE_PAYWALL_TIMEOUT=24h
STORE_PAYWALL_MIN_CONFIRMATIONS=1

# Payment Modes
STORE_PAYMENT_MODE_DIGITAL=single-sig
STORE_PAYMENT_MODE_SHIPPING=multisig-escrow
STORE_PAYMENT_MODE_POD=single-sig

# Multisig/Escrow (optional, for physical goods)
STORE_MULTISIG_ENABLED=false
STORE_SELLER_PUBLIC_KEY=""
STORE_ARBITER_PUBLIC_KEY=""
STORE_SELLER_PRIVATE_KEY=""
STORE_ESCROW_TIMETESTNET` | No | Use Bitcoin testnet (default: true) |
| `STORE_PAYWALL_DB_PATH` | No | Path to paywall database (default: ./data/paywall.db) |
| `STORE_PAYWALL_TIMEOUT` | No | Payment timeout duration (default: 24h) |
| `STORE_MULTISIG_ENABLED` | No | Enable multisig escrow for physical goods (default: false) |
| `STORE_SELLER_PUBLIC_KEY` | No | Seller's Bitcoin public key (hex) for escrow |
| `STORE_ARBITER_PUBLIC_KEY` | No | Arbiter's Bitcoin public key (hex) for disputes |
| `STORE_SELLER_PRIVATE_KEY` | No | Seller's Bitcoin private key (encrypted) |
| `STORE_ESCROW_TIMEOUT_PHYSICAL` | No | Escrow timeout for physical goods (default: 168h) |
| `STORE_PUBLIC_URL` | Yes | Public URL of this store (for callbacks) |
| `STORE_AUTO_FULFILL` | No | Auto-fulfill payments after confirmation (default: true) |
| `STORE_ADMIN_TOKEN` | Yes | Token for admin API authentication |
| `STORE_ENCRYPTION_KEY` | No | Base64-encoded 32-byte key for encrypting configs |
| `STORE_RATE_LIMIT_ENABLED` | No | Enable rate limiting on checkout (default: true) |
| `STORE_LOG_LEVEL` | No | Log level: debug, info, warn, error (default: info
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
| `STORE_ENCRYPTION_KEY` | No | Base64-encoded 32-byte key for encrypting backend configs (optional) |
| `STORE_RATE_LIMIT_ENABLED` | No | Enable rate limiting on checkout (default: true) |
| `STORE_RATE_LIMIT_REQUESTS` | No | Rate limit requests per window (default: 5) |
| `STORE_RATE_LIMIT_BURST` | No | Rate limit burst allowance (default: 5) |
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
    "download_url": "/api/download/pay_def456",
    "expires_at": "2026-05-14T16:05:00Z",
    "max_downloads": 10
  },
  "fulfilled_at": "2026-05-13T16:10:00Z"
}
```

### Download Digital Content
Once payment is fulfilled, use the `download_url` from the fulfillment result to download the file:

```bash
curl -o go-programming.pdf http://localhost:8080/api/download/pay_def456
```

**Response:** The file is served with appropriate headers for download:
- `Content-Disposition: attachment; filename="go-programming.pdf"`
- `Content-Type: application/octet-stream`
- `Content-Length: <file_size>`

**Download tracking and limits:**
- Each download is recorded with timestamp, IP address, and user agent
- If `max_downloads` is configured, the endpoint returns `429 Too Many Requests` when the limit is exceeded
- Expired links return `410 Gone` with an error message
- Only fulfilled payments can be downloaded; unfulfilled payments return `403 Forbidden`

**Example error responses:**

Download limit exceeded:
```json
{
  "error": "Download limit exceeded"
}
```

Expired link:
```json
{
  "error": "Download link has expired"
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

### S3 Storage Configuration

The `digital_media` handler supports Amazon S3 for file storage with pre-signed URLs. To enable S3 storage:

#### 1. Configure AWS Credentials

Set environment variables for AWS access:
```bash
export AWS_ACCESS_KEY_ID=your_access_key
export AWS_SECRET_ACCESS_KEY=your_secret_key
export AWS_REGION=us-east-1
```

Or use IAM instance roles when running on EC2/ECS (recommended for production).

#### 2. Create S3 Bucket

```bash
aws s3 mb s3://your-store-downloads --region us-east-1
```

#### 3. Set IAM Policy

Attach the following policy to your IAM user/role:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "s3:GetObject",
        "s3:HeadObject"
      ],
      "Resource": "arn:aws:s3:::your-store-downloads/*"
    },
    {
      "Effect": "Allow",
      "Action": [
        "s3:ListBucket"
      ],
      "Resource": "arn:aws:s3:::your-store-downloads"
    }
  ]
}
```

**Minimalpermissions:** The store only needs `s3:GetObject` and `s3:HeadObject` for serving downloads. Upload files separately using AWS CLI, S3 console, or a separate upload process.

#### 4. Configure Item with S3 Storage

```bash
curl -X POST http://localhost:8080/admin/items \
  -H "Content-Type: application/json" \
  -H "X-Admin-Token: your-secret-token" \
  -d '{
    "name": "Advanced Go Course",
    "price": "0.005",
    "currency": "BTC",
    "category_id": "cat_abc123",
    "backend_type": "digital_media",
    "backend_config": {
      "storage": "s3",
      "s3_bucket": "your-store-downloads",
      "s3_key": "courses/advanced-go.zip",
      "s3_region": "us-east-1",
      "expiration_hours": 48,
      "max_downloads": 5
    }
  }'
```

**Configuration fields:**
- `storage`: Must be `"s3"`
- `s3_bucket`: Your S3 bucket name
- `s3_key`: Object key (path) within the bucket
- `s3_region`: AWS region (e.g., `us-east-1`, `eu-west-1`)
- `expiration_hours`: How long pre-signed URLs remain valid (minimum: 1, maximum: 168)
- `max_downloads`: Optional download limit per payment

**S3 download flow:**
1. Customer completes payment
2. Store fulfills payment by generating pre-signed S3 URL (valid for configured hours)
3. Download endpoint (`/api/download/{payment_id}`) redirects to pre-signed URL
4. Customer downloads directly from S3 (store doesn't proxy the file)
5. Download is tracked; limit enforced if configured

**Troubleshooting:**
- "Access Denied" errors: Check IAM permissions and bucket policy
- "Bucket not found": Verify region matches `s3_region` in config
- "Invalid credentials": Ensure `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY` are set

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

## Security

### Backend Configuration Encryption

opd-ai/store supports optional encryption of sensitive backend configuration data (API keys, webhooks, credentials) using AES-256-GCM. When enabled, all `backend_config` fields are encrypted at rest in the database.

**Enable encryption:**

1. Generate an encryption key:
```bash
make generate-key
# Or manually:
go run ./cmd/rotate-key -generate
```

2. Add the generated key to your environment:
```bash
export STORE_ENCRYPTION_KEY=<base64-key>
```

3. Restart the store service. New items will have encrypted backend configurations.

**Key rotation:**

To rotate encryption keys without downtime:

```bash
# Using Makefile
make rotate-key OLD_KEY=<old-key> NEW_KEY=<new-key>

# Or directly
go run ./cmd/rotate-key -old-key=<old-key> -new-key=<new-key> -db=./data/store.db
```

The rotation tool:
- Decrypts all items with the old key
- Re-encrypts with the new key
- Updates the database atomically
- Supports migrating from plaintext to encrypted

**Backward compatibility:**

- Without `STORE_ENCRYPTION_KEY`, configs are stored as plaintext
- With encryption enabled, the system can read both encrypted and plaintext configs
- Plaintext configs are automatically encrypted on the next update

**Important:**
- **Back up your encryption key securely** - lost keys cannot decrypt data
- Store the key separately from the database (environment, secrets manager, vault)
- Use different keys for development, staging, and production

### Webhook Security

Payment confirmation webhooks from opd-ai/paywall are verified using HMAC-SHA256 signatures. Configure the webhook secret:

```bash
export STORE_PAYWALL_WEBHOOK_SECRET=<shared-secret>
```

Webhooks without valid signatures are rejected with HTTP 401 Unauthorized.

### Rate Limiting

The checkout endpoint is rate-limited to 5 requests per minute per IP address by default. Configure with:

```bash
export STORE_RATE_LIMIT_REQUESTS=10    # Requests per window
export STORE_RATE_LIMIT_BURST=5        # Burst allowance
```

Disable rate limiting (not recommended for production):
```bash
export STORE_RATE_LIMIT_ENABLED=false
```

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
