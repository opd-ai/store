# Architecture Diagrams

## 1. Payment & Fulfillment Flow

```
          ┌─────────────────────────────────────────────────────────┐
          │                    CUSTOMER                             │
          └─────────────┬───────────────────────────────────────────┘
                        │
                        ▼
          ┌─────────────────────────────┐
          │   Store Frontend (Web)       │
          │ - Browse Catalog            │
          │ - View Items & Prices       │
          │ - Add to Cart               │
          └────────┬────────────────────┘
                   │ Checkout Request
                   ▼
          ┌─────────────────────────────┐
          │    Store API - POST /checkout
          │  (Create Payment Record)    │
          └────────┬────────────────────┘
                   │ Payment ID
                   ▼
          ┌──────────────────────────────────────┐
          │  opd-ai/paywall (External Service)   │
          │ - Generate Invoice                   │
          │ - Display QR Code                    │
          │ - Listen for Payment                 │
          └────────┬─────────────────────────────┘
                   │ Payment Confirmed + TX Hash
                   ▼
          ┌──────────────────────────────────────┐
          │ Store API - POST /admin/payment/:id  │
          │         /confirm                     │
          │  (Mark Payment as Confirmed)         │
          └────────┬─────────────────────────────┘
                   │
                   ▼
          ┌──────────────────────────────────────────────────┐
          │     FulfillmentHandler Registry                  │
          │  (Dynamic Handler Dispatch by Item.BackendType)  │
          └──────┬──────────────────────────────────────────┘
                 │
      ┌──────────┼──────────┬──────────┬──────────┐
      │          │          │          │          │
      ▼          ▼          ▼          ▼          ▼
   ┌─────┐  ┌──────┐  ┌────┐  ┌──────────┐  ┌────────┐
   │ DM  │  │ Form │  │ PoD│ │ Custom   │  │ Other  │
   │     │  │      │  │    │ │ Webhook  │  │ ...    │
   └─────┘  └──────┘  └────┘  └──────────┘  └────────┘

Where:
- DM = Digital Media (download link)
- Form = Shipping Form (address collection)
- PoD = Print-on-Demand (external API)
- Custom = Webhook invocation
```

## 2. Component Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │              HTTP API Layer (Gorilla Mux)              │  │
│  │  - GET /api/catalog        (Public)                   │  │
│  │  - POST /api/checkout      (Public)                   │  │
│  │  - GET  /api/payment/:id   (Public - Status Check)   │  │
│  │  - POST /admin/*           (Admin Endpoints)          │  │
│  └──────────────────────────────────────────────────────────┘  │
│                              │                                  │
│  ┌──────────────────────────▼─────────────────────────────┐    │
│  │          API Handler Layer (internal/api)             │    │
│  │  - Request parsing & validation                       │    │
│  │  - Admin token authentication                         │    │
│  │  - Response serialization                             │    │
│  └──────────────────────────▼─────────────────────────────┘    │
│                              │                                  │
│  ┌──────────────────────────▼─────────────────────────────┐    │
│  │       Store Service Layer (pkg/store)                 │    │
│  │  - CreatePayment()                                    │    │
│  │  - ConfirmPayment()                                   │    │
│  │  - FulfillPayment()                                   │    │
│  │  - SubmitFormData()                                   │    │
│  │  - GetCatalog()                                       │    │
│  └──────────────────────────▼─────────────────────────────┘    │
│                              │                                  │
│  ┌──────────────────────────▼──────────────────────────────┐   │
│  │   Handler Interface & Registry (pkg/handler)           │   │
│  │   FulfillmentHandler interface                         │   │
│  │   - Handle(ctx, payment, item) -> result              │   │
│  │   - Validate(config) -> error                         │   │
│  │   - Metadata() -> HandlerMetadata                      │   │
│  └───┬──────────────────────────────────────────────────┬──┘   │
│     │                                                    │      │
│     ├─► DigitalMediaHandler (internal/handlers)        │      │
│     ├─► ShippingFormHandler (internal/handlers)        │      │
│     ├─► PrintOnDemandHandler (internal/handlers)       │      │
│     └─► CustomHandler (internal/handlers)              │      │
│                                                         │      │
│  ┌──────────────────────────────────────────────────────▼──┐  │
│  │           Data Models (pkg/models)                      │  │
│  │  - Category, Tag, Item                                 │  │
│  │  - Payment, FormSubmission                             │  │
│  │  - JSONMap (custom JSON types)                         │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │    BoltDB Embedded Database (bbolt)                      │  │
│  │  - categories, items, tags, payments, form_submissions  │  │
│  │  - Buckets with JSON encoding                           │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

## 3. Handler Dispatch Flow

```
┌─────────────────────────────────┐
│  FulfillPayment() Invoked       │
│  (after payment confirmed)      │
└────────────┬────────────────────┘
             │
             ▼
┌─────────────────────────────────┐
│ Fetch Payment + Item from DB    │
└────────────┬────────────────────┘
             │
             ▼
┌──────────────────────────────────────┐
│ registry.Get(item.BackendType) ?     │
└─┬──────┬──────┬──────┬───────────────┘
  │      │      │      │
  ▼      ▼      ▼      ▼
 DM    Form   PoD   Custom

 ┌─────────────────────────────────┐ 
 │  Selected Handler.Handle()      │ 
 │  (ctx, payment, item)           │ 
 └────────────┬────────────────────┘ 
              │ 
              ▼ 
 ┌──────────────────────────────────────────┐ 
 │  Backend-Specific Logic                  │ 
 │  (generate URL, form, webhook call, etc) │ 
 └────────────┬─────────────────────────────┘ 
              │ 
              ▼ 
 ┌──────────────────────────────────────────┐ 
 │  Result: map[string]interface{}          │ 
 │  e.g. {"download_url": "...", ...}      │ 
 └────────────┬─────────────────────────────┘ 
              │ 
              ▼ 
 ┌──────────────────────────────────────────┐ 
 │  Payment.Fulfill(result)                 │ 
 │  Update DB: status="fulfilled"           │ 
 └──────────────────────────────────────────┘ 
```

## 4. BoltDB Bucket Structure

```
BoltDB Database (store.db)
│
├─ categories          (Bucket)
│  └─ {id} → Category JSON
│
├─ tags               (Bucket)
│  └─ {id} → Tag JSON
│
├─ items              (Bucket)
│  └─ {id} → Item JSON (includes category_id, backend_type, backend_config)
│
├─ payments           (Bucket)
│  └─ {id} → Payment JSON (includes item_id, amount, status, fulfillment_result)
│
├─ form_submissions   (Bucket)
│  └─ {id} → FormSubmission JSON (includes payment_id, form_data)
│
├─ download_logs      (Bucket)
│  └─ {id} → DownloadLog JSON
│
├─ payments_by_invoice    (Index Bucket)
│  └─ {invoice_id} → payment_id
│
├─ payments_by_status     (Index Bucket)
│  └─ {status}:{id} → payment_id
│
├─ items_by_category      (Index Bucket)
│  └─ {category_id}:{id} → item_id
│
├─ item_tags              (Index Bucket - Many-to-Many)
│  └─ {item_id}:{tag_id} → association
│
├─ tag_items              (Index Bucket - Many-to-Many)
│  └─ {tag_id}:{item_id} → association
│
└─ downloads_by_payment   (Index Bucket)
   └─ {payment_id}:{timestamp} → log_id

All data stored as JSON-encoded values.
Keys are string-based identifiers.
```

## 5. Deployment Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     Docker Compose                          │
│                                                             │
│  ┌──────────────┐             ┌─────────────────────────┐   │
│  │ Mock Paywall │             │   Store Service         │   │
│  │              │             │                         │   │
│  │  - Create    │             │  - API Handlers         │   │
│  │    Payment   │             │  - Handler Registry     │   │
│  │  - Verify TX │             │  - BoltDB (Embedded)    │   │
│  │              │             │    └─ store.db          │   │
│  └──────┬───────┘             └────────┬────────────────┘   │
│         │                              │                    │
│         └──────────────────────────────┘                    │
│                                                             │
│                Network: store_network                       │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐   │
│  │      Volumes                                        │   │
│  │  - store_data (BoltDB file + uploads persistence)  │   │
│  │  - templates/ (custom HTML/CSS)                    │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐   │
│  │      Ports                                          │   │
│  │  - localhost:8080 -> Store API                     │   │
│  │  - localhost:8081 -> Paywall Mock                  │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

## 6. Handler Configuration Flow

```
Admin Creates Item
        │
        ▼
┌─────────────────────────────────┐
│ POST /admin/items               │
│ {                               │
│   "name": "...",                │
│   "backend_type": "digital_media"│
│   "backend_config": {           │
│     "file_path": "/...",        │
│     "storage": "s3"             │
│   }                             │
│ }                               │
└────────┬────────────────────────┘
         │
         ▼
┌──────────────────────────────────────┐
│ API Handler validates config         │
│ via handler.Validate()               │
└────────┬─────────────────────────────┘
         │
      Success/Error
         │
      ┌──┴──┐
      ▼     ▼
   Save   Reject
    to     with
    DB    error

Later... Payment Confirmation
         │
         ▼
   ┌─────────────────┐
   │ handler.Handle()│
   │ (uses config)   │
   └─────────────────┘
```

## 7. API Endpoint Summary

### Public Endpoints
```
GET  /health
GET  /api/catalog                      → All items & categories
GET  /api/items/{id}                   → Single item details
POST /api/checkout                     → Create payment
GET  /api/payment/{id}/status          → Check payment status
POST /api/payment/{id}/submit-form     → Submit form data
GET  /admin/handlers                   → Registered handlers
```

### Admin Endpoints (require X-Admin-Token header)
```
POST   /admin/categories               → Create category
GET    /admin/categories               → List categories
PUT    /admin/categories/{id}          → Update category
DELETE /admin/categories/{id}          → Delete category

POST   /admin/items                    → Create item
GET    /admin/items                    → List items
PUT    /admin/items/{id}               → Update item
DELETE /admin/items/{id}               → Delete item

GET    /admin/payments                 → List payments
POST   /admin/payment/{id}/confirm     → Confirm payment
POST   /admin/payment/{id}/fulfill     → Trigger fulfillment
```
