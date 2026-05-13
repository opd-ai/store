# Deployment Guide

## Overview

This guide covers deploying opd-ai/store to production environments. Whether you're running on a VPS, Kubernetes, or a cloud provider, this guide will help you set up a secure, scalable, and maintainable deployment.

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Deployment Options](#deployment-options)
3. [Environment Configuration](#environment-configuration)
4. [Database Setup](#database-setup)
5. [Security Checklist](#security-checklist)
6. [Deployment Methods](#deployment-methods)
7. [Monitoring and Logging](#monitoring-and-logging)
8. [Backup and Recovery](#backup-and-recovery)
9. [Performance Tuning](#performance-tuning)
10. [Troubleshooting](#troubleshooting)

## Prerequisites

Before deploying, ensure you have:

- ✅ Go 1.21+ (for building from source)
- ✅ PostgreSQL 15+ database
- ✅ opd-ai/paywall service running (or plan to deploy it)
- ✅ Domain name with SSL certificate (Let's Encrypt recommended)
- ✅ Server with at least 512MB RAM and 1GB storage
- ✅ Access to SMTP server (for email notifications, if using email handler)

## Deployment Options

### Option 1: Docker (Recommended)

Best for: Simple deployments, quick setup, isolated environments

**Pros:**
- Easy to deploy and update
- Consistent environment
- Minimal configuration

**Cons:**
- Requires Docker knowledge
- Additional overhead

### Option 2: Systemd Service

Best for: VPS deployments, minimal overhead, traditional Linux servers

**Pros:**
- Native system integration
- Lower resource usage
- Better performance

**Cons:**
- Manual setup required
- OS-specific

### Option 3: Kubernetes

Best for: Large-scale deployments, high availability, auto-scaling

**Pros:**
- Horizontal scaling
- Self-healing
- Rolling updates

**Cons:**
- Complex setup
- Higher resource requirements
- Steeper learning curve

## Environment Configuration

### Required Variables

Create a `.env` file or configure these environment variables:

```bash
# Database
STORE_DATABASE_URL=postgres://user:password@localhost:5432/store_production

# Server
STORE_PORT=8080
STORE_HOST=0.0.0.0
STORE_PUBLIC_URL=https://store.example.com

# Paywall Integration (opd-ai/paywall)
STORE_PAYWALL_URL=https://paywall.example.com
STORE_PAYWALL_API_KEY=sk_live_your_api_key_here
STORE_PAYWALL_WEBHOOK_SECRET=whsec_your_webhook_secret_here

# Admin Authentication
STORE_ADMIN_TOKEN=your_secure_random_token_here

# Fulfillment
STORE_AUTO_FULFILL=true

# Storage (for digital media handler)
STORE_UPLOADS_DIR=/var/lib/store/uploads

# Logging
STORE_LOG_LEVEL=info
STORE_LOG_FORMAT=json
```

### Optional Variables

```bash
# Templates (for custom handler)
STORE_TEMPLATES_DIR=/etc/store/templates

# AWS S3 (for digital media handler with S3 storage)
AWS_ACCESS_KEY_ID=your_access_key
AWS_SECRET_ACCESS_KEY=your_secret_key
AWS_REGION=us-east-1

# Print-on-Demand (for pod handler)
PRINTFUL_API_KEY=your_printful_api_key
```

### Generating Secure Tokens

Generate secure random tokens for production:

```bash
# Admin token (64 characters)
openssl rand -hex 32

# Webhook secret (32 characters)
openssl rand -hex 16
```

## Database Setup

### Production Database Configuration

1. **Create Database:**

```sql
CREATE DATABASE store_production;
CREATE USER store_user WITH ENCRYPTED PASSWORD 'secure_password_here';
GRANT ALL PRIVILEGES ON DATABASE store_production TO store_user;
```

2. **Connection Pooling:**

For production, use connection pooling in your connection string:

```
postgres://store_user:password@localhost:5432/store_production?pool_max_conns=25&pool_min_conns=5
```

3. **SSL/TLS:**

For remote databases, enable SSL:

```
postgres://store_user:password@db.example.com:5432/store_production?sslmode=require
```

### Database Migrations

opd-ai/store uses GORM's auto-migration feature. On startup, it automatically creates/updates tables.

**Migration happens at startup** in `cmd/store/main.go`:

```go
if err := db.AutoMigrate(
    &models.Category{},
    &models.Tag{},
    &models.Item{},
    &models.Payment{},
    &models.FormSubmission{},
    &models.DownloadLog{},
); err != nil {
    log.Fatalf("Failed to run migrations: %v", err)
}
```

**For production, consider:**

1. **Test migrations in staging first**
2. **Backup database before deploying new versions**
3. **Use a separate migration tool** for complex changes (e.g., [golang-migrate](https://github.com/golang-migrate/migrate))

### Managed Database Services

Recommended providers:

- **AWS RDS PostgreSQL** - Automated backups, read replicas, multi-AZ
- **Google Cloud SQL** - Managed PostgreSQL with high availability
- **DigitalOcean Managed Databases** - Simple, affordable, automatic backups
- **Azure Database for PostgreSQL** - Enterprise-grade managed service
- **Supabase** - Open-source alternative with built-in API

## Security Checklist

### Before Going Live

- [ ] **Change all default credentials**
- [ ] **Use strong, unique `STORE_ADMIN_TOKEN`**
- [ ] **Enable HTTPS with valid SSL certificate**
- [ ] **Set `STORE_PUBLIC_URL` to HTTPS domain**
- [ ] **Restrict database access** (firewall rules, private network)
- [ ] **Use environment variables**, not hardcoded secrets
- [ ] **Enable database SSL/TLS**
- [ ] **Set up webhook signature verification**
- [ ] **Configure CORS appropriately** (not `*` in production)
- [ ] **Use read-only database credentials** where possible
- [ ] **Enable rate limiting** on public endpoints (see Nginx example)
- [ ] **Set up monitoring and alerting**
- [ ] **Review handler configurations** for sensitive data

### SSL/TLS Configuration

Use a reverse proxy (Nginx or Caddy) for SSL termination.

**Nginx Example:**

```nginx
server {
    listen 443 ssl http2;
    server_name store.example.com;

    ssl_certificate /etc/letsencrypt/live/store.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/store.example.com/privkey.pem;
    
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;
    ssl_prefer_server_ciphers on;

    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}

server {
    listen 80;
    server_name store.example.com;
    return 301 https://$server_name$request_uri;
}
```

**Caddy Example (automatic HTTPS):**

```
store.example.com {
    reverse_proxy localhost:8080
}
```

## Deployment Methods

### Method 1: Docker Compose (Recommended for Single Server)

**Step 1: Create production docker-compose.yml**

```yaml
version: '3.8'

services:
  postgres:
    image: postgres:15-alpine
    restart: unless-stopped
    environment:
      POSTGRES_DB: store_production
      POSTGRES_USER: store_user
      POSTGRES_PASSWORD: ${DB_PASSWORD}
    volumes:
      - postgres_data:/var/lib/postgresql/data
    networks:
      - internal
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U store_user"]
      interval: 10s
      timeout: 5s
      retries: 5

  store:
    image: ghcr.io/opd-ai/store:latest
    restart: unless-stopped
    depends_on:
      postgres:
        condition: service_healthy
    environment:
      STORE_DATABASE_URL: postgres://store_user:${DB_PASSWORD}@postgres:5432/store_production
      STORE_PORT: 8080
      STORE_PUBLIC_URL: ${PUBLIC_URL}
      STORE_PAYWALL_URL: ${PAYWALL_URL}
      STORE_PAYWALL_API_KEY: ${PAYWALL_API_KEY}
      STORE_PAYWALL_WEBHOOK_SECRET: ${PAYWALL_WEBHOOK_SECRET}
      STORE_ADMIN_TOKEN: ${ADMIN_TOKEN}
      STORE_AUTO_FULFILL: "true"
      STORE_LOG_LEVEL: info
      STORE_LOG_FORMAT: json
    volumes:
      - store_uploads:/var/lib/store/uploads
    networks:
      - internal
      - web
    ports:
      - "127.0.0.1:8080:8080"

  nginx:
    image: nginx:alpine
    restart: unless-stopped
    depends_on:
      - store
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf:ro
      - ./ssl:/etc/nginx/ssl:ro
      - letsencrypt:/etc/letsencrypt
    networks:
      - web

volumes:
  postgres_data:
  store_uploads:
  letsencrypt:

networks:
  internal:
  web:
```

**Step 2: Create .env file**

```bash
DB_PASSWORD=your_secure_db_password
PUBLIC_URL=https://store.example.com
PAYWALL_URL=https://paywall.example.com
PAYWALL_API_KEY=sk_live_xxx
PAYWALL_WEBHOOK_SECRET=whsec_xxx
ADMIN_TOKEN=your_secure_admin_token
```

**Step 3: Deploy**

```bash
docker-compose up -d
docker-compose logs -f store
```

**Step 4: Verify**

```bash
curl https://store.example.com/health
```

### Method 2: Systemd Service

**Step 1: Build binary**

```bash
go build -o /usr/local/bin/store ./cmd/store
```

**Step 2: Create systemd service**

Create `/etc/systemd/system/store.service`:

```ini
[Unit]
Description=opd-ai/store - cryptocurrency payment store
After=network.target postgresql.service
Requires=postgresql.service

[Service]
Type=simple
User=store
Group=store
WorkingDirectory=/var/lib/store
EnvironmentFile=/etc/store/config.env
ExecStart=/usr/local/bin/store
Restart=always
RestartSec=10

# Security
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/store /var/log/store

# Resource limits
LimitNOFILE=65536
LimitNPROC=512

[Install]
WantedBy=multi-user.target
```

**Step 3: Create user and directories**

```bash
sudo useradd -r -s /bin/false store
sudo mkdir -p /var/lib/store/uploads
sudo mkdir -p /var/log/store
sudo mkdir -p /etc/store
sudo chown -R store:store /var/lib/store /var/log/store
```

**Step 4: Create environment file**

Create `/etc/store/config.env`:

```bash
STORE_DATABASE_URL=postgres://store_user:password@localhost:5432/store_production
STORE_PORT=8080
STORE_HOST=127.0.0.1
STORE_PUBLIC_URL=https://store.example.com
STORE_PAYWALL_URL=https://paywall.example.com
STORE_PAYWALL_API_KEY=sk_live_xxx
STORE_PAYWALL_WEBHOOK_SECRET=whsec_xxx
STORE_ADMIN_TOKEN=xxx
STORE_AUTO_FULFILL=true
STORE_UPLOADS_DIR=/var/lib/store/uploads
STORE_LOG_LEVEL=info
STORE_LOG_FORMAT=json
```

**Step 5: Start service**

```bash
sudo systemctl daemon-reload
sudo systemctl enable store
sudo systemctl start store
sudo systemctl status store
```

**Step 6: View logs**

```bash
sudo journalctl -u store -f
```

### Method 3: Kubernetes

**Step 1: Create namespace**

```bash
kubectl create namespace opd-store
```

**Step 2: Create secrets**

```bash
kubectl create secret generic store-secrets \
  --namespace=opd-store \
  --from-literal=db-password='your_db_password' \
  --from-literal=admin-token='your_admin_token' \
  --from-literal=paywall-api-key='sk_live_xxx' \
  --from-literal=paywall-webhook-secret='whsec_xxx'
```

**Step 3: Create deployment manifest**

Create `k8s/deployment.yaml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: store-config
  namespace: opd-store
data:
  STORE_PORT: "8080"
  STORE_HOST: "0.0.0.0"
  STORE_PUBLIC_URL: "https://store.example.com"
  STORE_PAYWALL_URL: "https://paywall.example.com"
  STORE_AUTO_FULFILL: "true"
  STORE_LOG_LEVEL: "info"
  STORE_LOG_FORMAT: "json"

---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: store
  namespace: opd-store
spec:
  replicas: 3
  selector:
    matchLabels:
      app: store
  template:
    metadata:
      labels:
        app: store
    spec:
      containers:
      - name: store
        image: ghcr.io/opd-ai/store:latest
        ports:
        - containerPort: 8080
          name: http
        envFrom:
        - configMapRef:
            name: store-config
        env:
        - name: STORE_DATABASE_URL
          value: "postgres://store_user:$(DB_PASSWORD)@postgres:5432/store_production"
        - name: DB_PASSWORD
          valueFrom:
            secretKeyRef:
              name: store-secrets
              key: db-password
        - name: STORE_ADMIN_TOKEN
          valueFrom:
            secretKeyRef:
              name: store-secrets
              key: admin-token
        - name: STORE_PAYWALL_API_KEY
          valueFrom:
            secretKeyRef:
              name: store-secrets
              key: paywall-api-key
        - name: STORE_PAYWALL_WEBHOOK_SECRET
          valueFrom:
            secretKeyRef:
              name: store-secrets
              key: paywall-webhook-secret
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
        resources:
          requests:
            memory: "128Mi"
            cpu: "100m"
          limits:
            memory: "512Mi"
            cpu: "500m"

---
apiVersion: v1
kind: Service
metadata:
  name: store
  namespace: opd-store
spec:
  selector:
    app: store
  ports:
  - port: 80
    targetPort: 8080
    name: http
  type: ClusterIP

---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: store
  namespace: opd-store
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
    nginx.ingress.kubernetes.io/rate-limit: "100"
spec:
  ingressClassName: nginx
  tls:
  - hosts:
    - store.example.com
    secretName: store-tls
  rules:
  - host: store.example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: store
            port:
              number: 80
```

**Step 4: Deploy**

```bash
kubectl apply -f k8s/deployment.yaml
kubectl get pods -n opd-store
kubectl logs -f deployment/store -n opd-store
```

## Monitoring and Logging

### Health Checks

The `/health` endpoint returns server status:

```bash
curl https://store.example.com/health
```

**Response:**
```json
{"status": "ok"}
```

### Structured Logging

Set `STORE_LOG_FORMAT=json` for structured logs:

```json
{
  "level": "info",
  "time": "2026-05-13T16:00:00Z",
  "message": "Starting server on port 8080"
}
```

### Log Aggregation

**Option 1: ELK Stack**
- Elasticsearch for storage
- Logstash for processing
- Kibana for visualization

**Option 2: Loki + Grafana**
- Lightweight alternative to ELK
- Integrates with Grafana

**Option 3: Cloud Services**
- AWS CloudWatch Logs
- Google Cloud Logging
- Datadog
- New Relic

### Metrics

Add Prometheus metrics (future enhancement):

```go
import "github.com/prometheus/client_golang/prometheus"

var (
    paymentsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "store_payments_total",
            Help: "Total number of payments",
        },
        []string{"status", "currency"},
    )
)
```

### Alerting

Set up alerts for:
- ❌ Service down (health check failures)
- ⚠️ Database connection errors
- ⚠️ Paywall API errors
- ⚠️ High error rates
- ⚠️ Slow response times
- ⚠️ Disk space low (uploads directory)

## Backup and Recovery

### Database Backups

**Automated Daily Backups:**

```bash
#!/bin/bash
# /usr/local/bin/backup-store-db.sh

DATE=$(date +%Y%m%d_%H%M%S)
BACKUP_DIR="/var/backups/store"
DB_NAME="store_production"

# Create backup directory
mkdir -p "$BACKUP_DIR"

# Dump database
pg_dump -U store_user -h localhost "$DB_NAME" | gzip > "$BACKUP_DIR/store_$DATE.sql.gz"

# Keep only last 30 days
find "$BACKUP_DIR" -name "store_*.sql.gz" -mtime +30 -delete

# Upload to S3 (optional)
aws s3 cp "$BACKUP_DIR/store_$DATE.sql.gz" "s3://my-backups/store/"
```

**Cron Job:**

```cron
0 2 * * * /usr/local/bin/backup-store-db.sh
```

### Restore from Backup

```bash
# Decompress backup
gunzip store_20260513_020000.sql.gz

# Restore database
psql -U store_user -h localhost store_production < store_20260513_020000.sql
```

### Uploads Backup

Backup the uploads directory regularly:

```bash
# Rsync to backup server
rsync -avz /var/lib/store/uploads/ backup-server:/backups/store/uploads/

# Or tar and upload to S3
tar -czf uploads_$(date +%Y%m%d).tar.gz /var/lib/store/uploads
aws s3 cp uploads_$(date +%Y%m%d).tar.gz s3://my-backups/store/
```

### Disaster Recovery Plan

1. **Document all credentials** in a secure location (password manager)
2. **Test restore procedures** quarterly
3. **Keep backups in multiple locations** (on-site + off-site)
4. **Automate backups** (don't rely on manual processes)
5. **Monitor backup success** (alert on failures)

## Performance Tuning

### Database Optimization

**Connection Pooling:**

```go
sqlDB, _ := db.DB()
sqlDB.SetMaxOpenConns(25)
sqlDB.SetMaxIdleConns(5)
sqlDB.SetConnMaxLifetime(time.Hour)
```

**Indexes:**

Ensure critical queries are indexed:

```sql
CREATE INDEX idx_payments_status ON payments(status);
CREATE INDEX idx_payments_item_id ON payments(item_id);
CREATE INDEX idx_items_backend_type ON items(backend_type);
CREATE INDEX idx_items_category_id ON items(category_id);
```

### Caching

For high-traffic deployments, add Redis caching for catalog:

```go
// Cache catalog for 5 minutes
cachedCatalog, err := redis.Get("catalog")
if err == nil {
    return cachedCatalog
}

catalog := fetchCatalogFromDB()
redis.Set("catalog", catalog, 5*time.Minute)
```

### Rate Limiting

Use Nginx for rate limiting:

```nginx
http {
    limit_req_zone $binary_remote_addr zone=api:10m rate=10r/s;

    server {
        location /api/ {
            limit_req zone=api burst=20 nodelay;
            proxy_pass http://localhost:8080;
        }
    }
}
```

### CDN

Serve static assets via CDN:
- API documentation (`/api/docs`)
- Item images
- Download files (for digital media)

## Troubleshooting

### Common Issues

**1. Database Connection Failures**

```
Error: failed to connect to database: connection refused
```

**Solution:**
- Check `STORE_DATABASE_URL` is correct
- Verify PostgreSQL is running
- Check firewall rules
- Test connection: `psql postgres://user:pass@host:port/db`

**2. Paywall Integration Errors**

```
Error: failed to create invoice: connection timeout
```

**Solution:**
- Verify `STORE_PAYWALL_URL` is correct and reachable
- Check `STORE_PAYWALL_API_KEY` is valid
- Test paywall API directly: `curl $PAYWALL_URL/health`

**3. Webhook Signature Verification Fails**

```
Error: invalid webhook signature
```

**Solution:**
- Ensure `STORE_PAYWALL_WEBHOOK_SECRET` matches opd-ai/paywall configuration
- Check webhook payload is not modified by proxy
- Verify webhook URL is accessible from opd-ai/paywall

**4. Out of Memory**

```
fatal error: out of memory
```

**Solution:**
- Increase server RAM
- Reduce database connection pool size
- Check for memory leaks (use `go tool pprof`)

### Debug Mode

Enable debug logging:

```bash
STORE_LOG_LEVEL=debug go run cmd/store/main.go
```

### Health Diagnostics

```bash
# Check if server is responding
curl -v https://store.example.com/health

# Check database connectivity
psql $STORE_DATABASE_URL -c "SELECT 1"

# Check disk space
df -h /var/lib/store

# Check logs
journalctl -u store -n 100 --no-pager
```

## Post-Deployment Checklist

After deployment, verify:

- [ ] Health endpoint responds: `curl https://store.example.com/health`
- [ ] Admin API accessible: `curl -H "X-Admin-Token: ..." https://store.example.com/admin/handlers`
- [ ] Public catalog loads: `curl https://store.example.com/api/catalog`
- [ ] SSL certificate valid (no browser warnings)
- [ ] Logs are being collected
- [ ] Backups are running
- [ ] Monitoring/alerting configured
- [ ] Paywall integration working
- [ ] Test payment flow end-to-end
- [ ] Documentation updated with production URLs

## Updating

### Docker Compose

```bash
docker-compose pull
docker-compose up -d
docker-compose logs -f store
```

### Systemd

```bash
# Build new binary
go build -o /usr/local/bin/store ./cmd/store

# Restart service
sudo systemctl restart store
sudo systemctl status store
```

### Kubernetes

```bash
kubectl set image deployment/store store=ghcr.io/opd-ai/store:v1.1.0 -n opd-store
kubectl rollout status deployment/store -n opd-store
```

### Zero-Downtime Deployments

For Kubernetes, use rolling updates (default).

For systemd, use a blue-green deployment or load balancer with health checks.

## Support

- **Documentation**: https://github.com/opd-ai/store
- **Issues**: https://github.com/opd-ai/store/issues
- **Security**: security@opd-ai.com

---

*Need help? Open an issue or consult the [troubleshooting guide](../troubleshooting/README.md).*
