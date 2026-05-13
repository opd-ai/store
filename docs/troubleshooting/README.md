# Troubleshooting Guide

## Overview

This guide helps you diagnose and resolve common issues with opd-ai/store. Issues are organized by category with symptoms, causes, and solutions.

## Table of Contents

1. [Database Issues](#database-issues)
2. [Paywall Integration Issues](#paywall-integration-issues)
3. [Handler Failures](#handler-failures)
4. [Payment Confirmation Problems](#payment-confirmation-problems)
5. [API Errors](#api-errors)
6. [Performance Issues](#performance-issues)
7. [Configuration Problems](#configuration-problems)
8. [Debugging Tools](#debugging-tools)

---

## Database Issues

### Symptom: "Failed to connect to database: connection refused"

**Possible Causes:**
1. PostgreSQL is not running
2. Incorrect connection string
3. Firewall blocking connection
4. Database doesn't exist

**Solutions:**

**1. Verify PostgreSQL is running:**
```bash
# Check if PostgreSQL is running
sudo systemctl status postgresql

# Start if not running
sudo systemctl start postgresql
```

**2. Test connection manually:**
```bash
psql "postgres://user:pass@localhost:5432/store_db"
```

If connection fails, check:
- Username and password are correct
- Database exists: `psql -U postgres -l`
- Host and port are correct

**3. Check firewall:**
```bash
# Allow PostgreSQL through firewall
sudo ufw allow 5432/tcp
```

**4. Verify `STORE_DATABASE_URL`:**
```bash
echo $STORE_DATABASE_URL
# Should be: postgres://user:password@host:port/database
```

---

### Symptom: "Failed to run migrations: relation already exists"

**Possible Causes:**
1. Migrations running multiple times simultaneously
2. Manual schema changes conflicting with auto-migrate
3. Partial migration failure

**Solutions:**

**1. Check for duplicate processes:**
```bash
# Ensure only one instance is running
ps aux | grep store
```

**2. Manually fix schema:**
```sql
-- Connect to database
psql $STORE_DATABASE_URL

-- Check existing tables
\dt

-- Drop conflicting table (CAUTION: data loss)
DROP TABLE IF EXISTS problem_table CASCADE;

-- Let auto-migrate recreate it
```

**3. Use transaction-safe migrations:**

For production, consider using [golang-migrate](https://github.com/golang-migrate/migrate) instead of auto-migrate.

---

### Symptom: "Too many connections" / "Connection pool exhausted"

**Possible Causes:**
1. Connection leak (connections not being closed)
2. Pool size too small for load
3. Long-running queries

**Solutions:**

**1. Check active connections:**
```sql
SELECT count(*) FROM pg_stat_activity WHERE datname = 'store_db';
```

**2. Increase pool size:**

In your database connection string:
```
postgres://user:pass@host:port/db?pool_max_conns=50&pool_min_conns=10
```

Or programmatically:
```go
sqlDB, _ := db.DB()
sqlDB.SetMaxOpenConns(50)
sqlDB.SetMaxIdleConns(10)
```

**3. Find slow queries:**
```sql
SELECT pid, now() - pg_stat_activity.query_start AS duration, query 
FROM pg_stat_activity 
WHERE state = 'active' AND now() - pg_stat_activity.query_start > interval '30 seconds';
```

**4. Kill stuck connections:**
```sql
SELECT pg_terminate_backend(pid) 
FROM pg_stat_activity 
WHERE datname = 'store_db' AND state = 'idle in transaction';
```

---

## Paywall Integration Issues

### Symptom: "Failed to create invoice: connection timeout"

**Possible Causes:**
1. `STORE_PAYWALL_URL` is incorrect or unreachable
2. Paywall service is down
3. Network issues or firewall blocking outbound connections
4. DNS resolution failure

**Solutions:**

**1. Verify paywall URL:**
```bash
echo $STORE_PAYWALL_URL
curl -v $STORE_PAYWALL_URL/health
```

**2. Check network connectivity:**
```bash
# Test DNS resolution
nslookup paywall.example.com

# Test connection
telnet paywall.example.com 443

# Check firewall
sudo iptables -L OUTPUT -n -v
```

**3. Verify paywall is running:**
```bash
# SSH to paywall server
ssh paywall-server

# Check status
sudo systemctl status paywall
journalctl -u paywall -n 50
```

**4. Use direct IP if DNS fails:**
```bash
STORE_PAYWALL_URL=http://192.168.1.100:8081
```

---

### Symptom: "Invalid API key" / "401 Unauthorized"

**Possible Causes:**
1. `STORE_PAYWALL_API_KEY` is incorrect
2. API key expired or revoked
3. API key not set

**Solutions:**

**1. Verify API key is set:**
```bash
# Check environment variable
echo $STORE_PAYWALL_API_KEY

# Should start with sk_live_ or sk_test_
```

**2. Test API key directly:**
```bash
curl -H "Authorization: Bearer $STORE_PAYWALL_API_KEY" \
     $STORE_PAYWALL_URL/api/v1/invoices
```

**3. Regenerate API key:**

Log into opd-ai/paywall admin panel and create a new API key. Update `STORE_PAYWALL_API_KEY` and restart.

---

### Symptom: "Webhook signature verification failed"

**Possible Causes:**
1. `STORE_PAYWALL_WEBHOOK_SECRET` doesn't match paywall configuration
2. Webhook payload modified by proxy
3. Timestamp drift (webhook too old)

**Solutions:**

**1. Verify webhook secret:**
```bash
echo $STORE_PAYWALL_WEBHOOK_SECRET
```

**2. Check paywall webhook configuration:**

Log into paywall admin and verify the webhook secret matches.

**3. Test webhook manually:**
```bash
# Generate test webhook (from paywall server)
curl -X POST https://store.example.com/webhook/payment-confirmed \
  -H "Content-Type: application/json" \
  -H "X-Paywall-Signature: test_signature" \
  -d '{
    "invoice_id": "inv_test123",
    "payment_hash": "abc123",
    "amount": "0.001",
    "currency": "BTC"
  }'
```

**4. Check for proxy modifications:**

If using Nginx/Caddy, ensure payload is not modified:
```nginx
location /webhook/ {
    proxy_pass http://localhost:8080;
    # Preserve body
    proxy_request_buffering off;
}
```

---

## Handler Failures

### Symptom: "Handler not registered: digital_media"

**Possible Causes:**
1. Handler not registered in `registerHandlers()`
2. Handler registration failed during startup
3. Typo in `backend_type`

**Solutions:**

**1. Check available handlers:**
```bash
curl -H "X-Admin-Token: your-token" http://localhost:8080/admin/handlers
```

**2. Verify handler registration:**

In `cmd/store/main.go`, ensure handler is in the list:
```go
handlersToRegister := []handler.FulfillmentHandler{
    handlers.NewDigitalMediaHandler(), // Must be here
    // ...
}
```

**3. Check startup logs:**
```bash
journalctl -u store -n 100 | grep "register"
```

**4. Use correct backend_type:**
```json
{
  "backend_type": "digital_media"  // Exact match required
}
```

---

### Symptom: "Fulfillment failed: S3 access denied"

**Possible Causes:**
1. AWS credentials not configured
2. S3 bucket doesn't exist
3. IAM permissions insufficient
4. Incorrect bucket region

**Solutions:**

**1. Verify AWS credentials:**
```bash
echo $AWS_ACCESS_KEY_ID
echo $AWS_SECRET_ACCESS_KEY
echo $AWS_REGION
```

**2. Test S3 access:**
```bash
aws s3 ls s3://your-bucket-name/
```

**3. Check IAM permissions:**

Required permissions:
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "s3:GetObject",
        "s3:PutObject",
        "s3:ListBucket"
      ],
      "Resource": [
        "arn:aws:s3:::your-bucket-name",
        "arn:aws:s3:::your-bucket-name/*"
      ]
    }
  ]
}
```

**4. Verify region:**
```bash
aws s3api get-bucket-location --bucket your-bucket-name
```

---

### Symptom: "Print-on-demand order creation failed"

**Possible Causes:**
1. Printful API key invalid
2. Product/variant ID doesn't exist
3. Shipping address incomplete
4. Out of stock

**Solutions:**

**1. Test Printful API key:**
```bash
curl -X GET "https://api.printful.com/store" \
  -H "Authorization: Bearer your_printful_api_key"
```

**2. Verify product exists:**
```bash
curl -X GET "https://api.printful.com/products/71" \
  -H "Authorization: Bearer your_printful_api_key"
```

**3. Check item configuration:**
```json
{
  "provider": "printful",
  "product_id": "71",        // Must be valid Printful product
  "variant_id": "4012",      // Must be valid variant
  "printful_api_key": "..."  // Must be set
}
```

**4. Enable debug logging:**
```bash
STORE_LOG_LEVEL=debug go run cmd/store/main.go
```

Check logs for detailed error from Printful API.

---

### Symptom: "Custom webhook invocation failed: connection refused"

**Possible Causes:**
1. Webhook URL is incorrect or unreachable
2. Webhook endpoint is down
3. Firewall blocking outbound connections
4. SSL certificate issues

**Solutions:**

**1. Test webhook URL:**
```bash
curl -X POST https://your-webhook.com/fulfill \
  -H "Content-Type: application/json" \
  -d '{"test": true}'
```

**2. Check SSL certificate:**
```bash
curl -v https://your-webhook.com/fulfill
# Look for "SSL certificate problem" errors
```

**3. Use HTTP instead of HTTPS for testing:**
```json
{
  "webhook_url": "http://your-webhook.com/fulfill"
}
```

**4. Check webhook logs:**

On the webhook server, check if requests are being received.

---

## Payment Confirmation Problems

### Symptom: "Payment stuck in 'pending' status"

**Possible Causes:**
1. Webhook not received from paywall
2. Payment not yet confirmed on blockchain
3. Webhook endpoint not accessible
4. Insufficient confirmations

**Solutions:**

**1. Check payment status on paywall:**
```bash
curl -H "Authorization: Bearer $PAYWALL_API_KEY" \
     $PAYWALL_URL/api/v1/invoices/{invoice_id}
```

**2. Manually confirm payment (admin):**
```bash
curl -X POST http://localhost:8080/admin/payment/{payment_id}/confirm \
  -H "X-Admin-Token: your-token" \
  -H "Content-Type: application/json" \
  -d '{"payment_hash": "actual_blockchain_hash"}'
```

**3. Check webhook logs:**
```bash
journalctl -u store | grep webhook
```

**4. Verify webhook URL is accessible:**
```bash
curl -X POST https://your-store.com/webhook/payment-confirmed
```

---

### Symptom: "Payment confirmed but not fulfilled"

**Possible Causes:**
1. `STORE_AUTO_FULFILL` is set to `false`
2. Handler fulfillment failed
3. Database transaction failed

**Solutions:**

**1. Check auto-fulfill setting:**
```bash
echo $STORE_AUTO_FULFILL
# Should be "true" for automatic fulfillment
```

**2. Manually trigger fulfillment:**
```bash
curl -X POST http://localhost:8080/admin/payment/{payment_id}/fulfill \
  -H "X-Admin-Token: your-token"
```

**3. Check fulfillment logs:**
```bash
journalctl -u store | grep -i fulfill
```

**4. Check payment status:**
```bash
curl http://localhost:8080/api/payment/{payment_id}/status
```

Look at `fulfillment_result` for error details.

---

### Symptom: "Duplicate payment confirmation"

**Possible Causes:**
1. Webhook sent multiple times
2. Manual confirmation after webhook
3. Idempotency not working

**Solutions:**

This is usually harmless - the handler should be idempotent. Check logs:
```bash
journalctl -u store | grep "already confirmed"
```

If causing issues, ensure webhook endpoint returns 200 on first attempt.

---

## API Errors

### Symptom: "401 Unauthorized" on admin endpoints

**Possible Causes:**
1. `X-Admin-Token` header missing
2. Token doesn't match `STORE_ADMIN_TOKEN`
3. Token not set in environment

**Solutions:**

**1. Verify token is set:**
```bash
echo $STORE_ADMIN_TOKEN
```

**2. Include token in request:**
```bash
curl -H "X-Admin-Token: your_secure_token" \
     http://localhost:8080/admin/categories
```

**3. Test token:**
```bash
# This should work
curl -H "X-Admin-Token: $STORE_ADMIN_TOKEN" \
     http://localhost:8080/admin/handlers

# This should fail
curl -H "X-Admin-Token: wrong_token" \
     http://localhost:8080/admin/handlers
```

---

### Symptom: "404 Not Found" on API endpoints

**Possible Causes:**
1. Incorrect endpoint URL
2. Route not registered
3. Typo in path

**Solutions:**

**1. Check available routes:**

Review `cmd/store/main.go` to see registered routes.

**2. Common endpoints:**
```
GET  /health
GET  /api/catalog
GET  /api/items/{id}
POST /api/checkout
GET  /api/payment/{id}/status
POST /admin/categories
GET  /admin/handlers
```

**3. Check server logs:**
```bash
journalctl -u store -f
```

Make a request and see if it appears in logs.

---

### Symptom: "500 Internal Server Error"

**Possible Causes:**
1. Database error
2. Handler panic
3. Configuration error
4. Bug in code

**Solutions:**

**1. Check server logs immediately:**
```bash
journalctl -u store -n 50
```

**2. Enable debug logging:**
```bash
STORE_LOG_LEVEL=debug
```

**3. Check database connectivity:**
```bash
psql $STORE_DATABASE_URL -c "SELECT 1"
```

**4. Restart service:**
```bash
sudo systemctl restart store
```

---

## Performance Issues

### Symptom: "Slow response times"

**Possible Causes:**
1. Database queries not indexed
2. Large catalog without caching
3. Insufficient resources
4. Network latency

**Solutions:**

**1. Add database indexes:**
```sql
CREATE INDEX idx_payments_status ON payments(status);
CREATE INDEX idx_payments_item_id ON payments(item_id);
CREATE INDEX idx_items_backend_type ON items(backend_type);
```

**2. Check database performance:**
```sql
-- Find slow queries
SELECT query, mean_exec_time, calls
FROM pg_stat_statements
ORDER BY mean_exec_time DESC
LIMIT 10;
```

**3. Monitor resource usage:**
```bash
# CPU and memory
top

# Disk I/O
iostat -x 1

# Network
iftop
```

**4. Scale up:**
- Increase server RAM/CPU
- Add database read replicas
- Use connection pooling
- Add caching layer (Redis)

---

### Symptom: "Memory leak / increasing memory usage"

**Possible Causes:**
1. Database connections not closed
2. Handler goroutine leak
3. Large file uploads not cleaned up

**Solutions:**

**1. Profile memory usage:**
```bash
go tool pprof http://localhost:6060/debug/pprof/heap
```

**2. Check for goroutine leaks:**
```bash
curl http://localhost:6060/debug/pprof/goroutine?debug=1
```

**3. Monitor memory over time:**
```bash
while true; do 
  ps aux | grep store | grep -v grep
  sleep 60
done
```

**4. Restart service regularly:**

As a temporary fix, set up a cron job to restart nightly:
```cron
0 3 * * * systemctl restart store
```

---

## Configuration Problems

### Symptom: "Environment variable not set"

**Possible Causes:**
1. `.env` file not loaded
2. Variable not exported in shell
3. Systemd service not using environment file

**Solutions:**

**1. For local development:**
```bash
# Load .env file
export $(cat .env | xargs)

# Or use godotenv (already used in main.go)
go run cmd/store/main.go
```

**2. For systemd:**

Ensure `EnvironmentFile` is set in `/etc/systemd/system/store.service`:
```ini
[Service]
EnvironmentFile=/etc/store/config.env
```

**3. For Docker:**

Pass environment variables:
```bash
docker run -e STORE_DATABASE_URL=postgres://... opd-ai/store
```

Or use env file:
```bash
docker run --env-file .env opd-ai/store
```

---

### Symptom: "Invalid configuration format"

**Possible Causes:**
1. JSON syntax error in `backend_config`
2. Missing required fields
3. Wrong data types

**Solutions:**

**1. Validate JSON:**
```bash
echo '{"storage":"local"}' | jq .
```

**2. Check handler requirements:**
```bash
curl -H "X-Admin-Token: your-token" \
     http://localhost:8080/admin/handlers
```

Look at `config_schema` for each handler.

**3. Test validation:**
```bash
# This will fail if config is invalid
curl -X POST http://localhost:8080/admin/items \
  -H "X-Admin-Token: your-token" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Test",
    "backend_type": "digital_media",
    "backend_config": {"invalid": "config"}
  }'
```

Error message will explain what's wrong.

---

## Debugging Tools

### Enable Debug Logging

```bash
STORE_LOG_LEVEL=debug go run cmd/store/main.go
```

Or for systemd:
```bash
sudo systemctl edit store
```

Add:
```ini
[Service]
Environment="STORE_LOG_LEVEL=debug"
```

### Check Server Health

```bash
curl http://localhost:8080/health
```

### Query Database Directly

```bash
psql $STORE_DATABASE_URL

-- Check recent payments
SELECT id, status, amount, currency, created_at 
FROM payments 
ORDER BY created_at DESC 
LIMIT 10;

-- Check items
SELECT id, name, backend_type, active 
FROM items 
WHERE active = true;

-- Check categories
SELECT id, name, slug FROM categories;
```

### Test Handlers

```bash
# List handlers
curl -H "X-Admin-Token: your-token" \
     http://localhost:8080/admin/handlers

# Check specific payment
curl http://localhost:8080/api/payment/{payment_id}/status
```

### Network Debugging

```bash
# Check listening ports
sudo netstat -tlnp | grep store

# Test connectivity
telnet localhost 8080

# Monitor traffic
sudo tcpdump -i any port 8080
```

### Log Analysis

```bash
# Tail logs in real-time
journalctl -u store -f

# Search for errors
journalctl -u store | grep -i error

# Filter by time
journalctl -u store --since "1 hour ago"

# Export to file
journalctl -u store -n 1000 > store_logs.txt
```

### Performance Profiling

Add to `main.go`:
```go
import _ "net/http/pprof"

func main() {
    go func() {
        log.Println(http.ListenAndServe("localhost:6060", nil))
    }()
    // ...
}
```

Then access:
```bash
# CPU profile
go tool pprof http://localhost:6060/debug/pprof/profile

# Memory profile
go tool pprof http://localhost:6060/debug/pprof/heap

# Goroutines
curl http://localhost:6060/debug/pprof/goroutine?debug=1
```

---

## Getting Help

### Before Opening an Issue

1. ✅ Check this troubleshooting guide
2. ✅ Search [existing issues](https://github.com/opd-ai/store/issues)
3. ✅ Enable debug logging and collect logs
4. ✅ Try to reproduce in a clean environment
5. ✅ Document exact steps to reproduce

### What to Include in Bug Reports

- **Version**: `git rev-parse HEAD` or Docker image tag
- **Environment**: OS, Go version, PostgreSQL version
- **Configuration**: Relevant environment variables (REDACT SECRETS)
- **Logs**: Full error messages and stack traces
- **Steps to reproduce**: Exact commands that trigger the issue
- **Expected behavior**: What should happen
- **Actual behavior**: What actually happens

### Support Channels

- **GitHub Issues**: https://github.com/opd-ai/store/issues
- **Documentation**: https://github.com/opd-ai/store
- **Security Issues**: security@opd-ai.com

---

## Quick Reference

### Common Commands

```bash
# Restart service
sudo systemctl restart store

# View logs
journalctl -u store -f

# Test database
psql $STORE_DATABASE_URL -c "SELECT 1"

# Test API
curl http://localhost:8080/health

# List handlers
curl -H "X-Admin-Token: $STORE_ADMIN_TOKEN" \
     http://localhost:8080/admin/handlers

# Check payment
curl http://localhost:8080/api/payment/{id}/status

# Manually confirm payment
curl -X POST http://localhost:8080/admin/payment/{id}/confirm \
  -H "X-Admin-Token: $STORE_ADMIN_TOKEN" \
  -d '{"payment_hash": "hash"}'

# Manually fulfill payment
curl -X POST http://localhost:8080/admin/payment/{id}/fulfill \
  -H "X-Admin-Token: $STORE_ADMIN_TOKEN"
```

### Log Levels

- `debug` - Verbose, for development
- `info` - Normal operation (default)
- `warn` - Warnings, non-critical
- `error` - Errors only

### Health Check Responses

| Status | Meaning |
|--------|---------|
| `{"status":"ok"}` | Healthy |
| Connection refused | Service down |
| Timeout | Service slow/hanging |

---

*Still stuck? Open an [issue](https://github.com/opd-ai/store/issues) with detailed information.*
