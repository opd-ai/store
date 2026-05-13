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
│  │  - JSONMap (custom GORM types)                         │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │    PostgreSQL Database (GORM ORM)                        │  │
│  │  - categories, items, tags, payments, form_submissions  │  │
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

## 4. Database Schema (Simplified)

```
┌──────────────┐         ┌───────────┐         ┌─────────┐
│  categories  │         │    tags   │         │  items  │
├──────────────┤         ├───────────┤         ├─────────┤
│ id (PK)      │         │ id (PK)   │         │ id (PK) │
│ name         │         │ name      │         │ cat_id (FK)
│ slug (U)     │         │ slug (U)  │         │ name    │
│ description  │         └───────────┘         │ price   │
│ order        │              △                 │ currency
│ metadata     │              │                 │ backend_type
│ created_at   │         ┌────┴─────┐           │ backend_config
│ updated_at   │         │           │           │ metadata
└──────────────┘    item_tags        │           │ active
      △           (join table)        │           │ created_at
      │                               │           │ updated_at
      │        ┌──────────────────────┘           └─────────┘
      │        │
  category_id──┘

┌──────────────┐         ┌──────────────────┐
│   payments   │         │  form_submissions│
├──────────────┤         ├──────────────────┤
│ id (PK)      │         │ id (PK)          │
│ item_id (FK) │         │ payment_id (FK)  │
│ amount       │         │ form_data        │
│ currency     │         │ submitted        │
│ payment_hash │         │ processed_at     │
│ status       │         │ created_at       │
│ payer_info   │         └──────────────────┘
│ confirmed_at │
│ fulfilled_at │
│ fulfillment_ │
│  result      │
│ created_at   │
│ updated_at   │
└──────────────┘
```

## 5. Deployment Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     Docker Compose                          │
│                                                             │
│  ┌────────────────┐  ┌──────────────┐  ┌─────────────────┐ │
│  │   PostgreSQL   │  │ Mock Paywall │  │  Store Service  │ │
│  │                │  │              │  │                 │ │
│  │  - Categories  │  │  - Create    │  │  - API Handlers │ │
│  │  - Items       │  │    Payment   │  │  - Handler Call │ │
│  │  - Payments    │  │  - Verify TX │  │  - DB Queries   │ │
│  │  - Forms       │  │              │  │                 │ │
│  └────────┬───────┘  └──────┬───────┘  └────────┬────────┘ │
│           │                 │                   │          │
│           └─────────────────┼───────────────────┘          │
│                             │                              │
│                Network: store_network                       │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐   │
│  │      Volumes                                        │   │
│  │  - postgres_data (DB persistence)                  │   │
│  │  - templates/ (custom HTML/CSS)                    │   │
│  │  - data/ (uploads, logs)                           │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐   │
│  │      Ports                                          │   │
│  │  - localhost:8080 -> Store API                     │   │
│  │  - localhost:5432 -> PostgreSQL                    │   │
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
