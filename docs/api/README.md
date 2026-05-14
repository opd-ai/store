# API Documentation

## Overview

This directory contains the OpenAPI 3.0 specification and interactive documentation for the opd-ai/store API.

## Accessing the Documentation

### Interactive Swagger UI

When the store server is running, access the interactive API documentation at:

```
http://localhost:8080/api/docs/
```

The Swagger UI provides:
- Complete API endpoint reference
- Request/response schemas
- Authentication requirements
- Try-it-out functionality for testing endpoints
- Filterable endpoint list

### OpenAPI Specification

The raw OpenAPI 3.0 specification is available at:

```
http://localhost:8080/api/docs/openapi.yaml
```

You can also view the file directly: [`openapi.yaml`](./openapi.yaml)

### Using the Specification

The OpenAPI specification can be used to:

1. **Generate Client Libraries**: Use [OpenAPI Generator](https://openapi-generator.tech/) to create client libraries in various languages
2. **Import into Postman**: Import the spec URL into Postman for quick API testing
3. **API Testing**: Use tools like [Dredd](https://dredd.org/) to validate API implementation against the spec
4. **Documentation Generation**: Generate static documentation with tools like [ReDoc](https://github.com/Redocly/redoc)

## API Structure

The API is organized into the following sections:

### Public Endpoints
- `/health` - Health check
- `/api/catalog` - Browse items and categories
- `/api/items/{id}` - Get item details
- `/api/checkout` - Create payment
- `/api/payment/{id}/status` - Check payment status
- `/api/payment/{id}/submit-form` - Submit fulfillment forms
- `/api/payment/{id}/download` - Track downloads

### Webhooks
- `/webhook/payment-confirmed` - Payment confirmation from opd-ai/paywall

### Admin Endpoints

**Categories, Items, Tags**:
- CRUD operations for managing catalog

**Payments**:
- List, confirm, and fulfill payments
- Manual intervention capabilities

**Orders**:
- Print-on-demand order status tracking

## Authentication

Admin endpoints require the `X-Admin-Token` header:

```bash
curl -H "X-Admin-Token: your-secret-token" http://localhost:8080/admin/categories
```

Set the token via the `STORE_ADMIN_TOKEN` environment variable.

## Examples

### Create a Category
```bash
curl -X POST http://localhost:8080/admin/categories \
  -H "Content-Type: application/json" \
  -H "X-Admin-Token: your-secret-token" \
  -d '{
    "name": "Digital Products",
    "description": "E-books and digital downloads"
  }'
```

### Create an Item
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

### Browse Catalog
```bash
curl http://localhost:8080/api/catalog
```

### Create Checkout
```bash
curl -X POST http://localhost:8080/api/checkout \
  -H "Content-Type: application/json" \
  -d '{
    "item_id": "item_xyz789",
    "email": "buyer@example.com"
  }'
```

## Development

### Updating the Specification

1. Edit [`openapi.yaml`](./openapi.yaml)
2. Restart the server
3. Refresh `/api/docs/` to see changes

### Validation

Validate the OpenAPI spec using:

```bash
# Using swagger-cli
npm install -g @apidevtools/swagger-cli
swagger-cli validate docs/api/openapi.yaml

# Using openapi-spec-validator
pip install openapi-spec-validator
openapi-spec-validator docs/api/openapi.yaml
```

## Rate Limiting

To prevent abuse and ensure fair resource allocation, the checkout endpoint is rate-limited.

**Default Limits**:
- **5 requests per minute** per IP address for `/api/checkout`
- Burst allowance: 5 requests
- Admin endpoints are exempt from rate limiting

**Rate Limit Headers**:

When rate limited, the API returns HTTP 429 Too Many Requests with a `Retry-After` header:

```http
HTTP/1.1 429 Too Many Requests
Retry-After: 60
Content-Type: application/json

{
  "error": "rate limit exceeded"
}
```

The `Retry-After` header indicates seconds to wait before retrying.

**Configuration**:

Adjust rate limits via environment variables:

```bash
export STORE_RATE_LIMIT_REQUESTS=10    # Requests per window
export STORE_RATE_LIMIT_BURST=5        # Burst allowance
export STORE_RATE_LIMIT_ENABLED=true   # Enable/disable (default: true)
```

**Best Practices**:
- Implement exponential backoff in your client
- Monitor the `Retry-After` header
- Cache catalog data to reduce repeated requests
- Use webhooks for payment confirmation instead of polling

## Additional Resources

- [OpenAPI 3.0 Specification](https://spec.openapis.org/oas/v3.0.3)
- [Swagger UI Documentation](https://swagger.io/tools/swagger-ui/)
- [OpenAPI Generator](https://openapi-generator.tech/)
