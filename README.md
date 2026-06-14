# loyalty-core-points

Go microservice — points engine and customer trazabilidad core for the loyalty platform.

Part of the **points vertical**: `web-points` → `bff-points` → **`core-points`**

Also consumed cross-vertically by `bff-ecommerce` (post-order accrual) and `bff-backoffice` (customer audit + program stats).

---

## Responsibilities

### Points engine
- **Accrue** points per customer (reference-linked, idempotent)
- **Redeem** points at checkout (balance check + atomic debit)
- **Balance** and **transaction history** per customer
- Program-wide aggregate stats: total points in circulation, lifetime accrued/redeemed

### Customer trazabilidad
- Persist and retrieve enrollment, password-change, and login traces
- Customer profile management (deterministic `customerId` from email hash)
- Lookup customer by email hash (used by login flow to resolve real `customerId`)

---

## Endpoints

### Points engine

| Method | Path | Description |
|---|---|---|
| `POST` | `/v1/points/accrue` | Accrue points for a customer |
| `POST` | `/v1/points/redeem` | Redeem points (with balance check) |
| `GET` | `/v1/points/:customerId/balance` | Current balance + lifetime stats |
| `GET` | `/v1/points/:customerId/transactions` | Transaction history (last 50) |

### Customer trazabilidad

| Method | Path | Description |
|---|---|---|
| `POST` | `/v1/customer-enrollments` | Create enrollment trace + auto-upsert customer profile |
| `GET` | `/v1/customer-enrollments` | List all enrollment traces |
| `GET` | `/v1/customer-enrollments/:transactionId` | Get enrollment trace by ID |
| `POST` | `/v1/customer-password-changes` | Create password change trace |
| `GET` | `/v1/customer-password-changes/:requestId` | Get password change trace |
| `POST` | `/v1/customer-logins` | Create login trace |
| `GET` | `/v1/customer-logins/:loginId` | Get login trace |

### Customer profiles

| Method | Path | Description |
|---|---|---|
| `GET` | `/v1/customers/:customerId/profile-summary` | Customer profile summary |
| `GET` | `/v1/customers/by-hash/:emailHash` | Lookup customerId by email hash |

### Observability

| Method | Path | Description |
|---|---|---|
| `GET` | `/health` | Health check with DB counts |
| `GET` | `/v1/stats` | Program stats: enrollments, logins, points in circulation |
| `GET` | `/metrics` | Prometheus metrics |

---

## Data model (Postgres)

```
point_accounts          — balance_points, lifetime_accrued, lifetime_redeemed per customer
point_transactions      — type (accrue|redeem), points, reference_id, source, description
customer_profiles       — customerId (cust_<hash12>), tier, enrollment/login/password status
customer_enrollment_traces     — transactionId, customerEmailHash, stage
customer_password_change_traces — requestId, transactionId, stage
customer_login_traces           — loginId, requestId, transactionId, stage
```

---

## Cross-vertical integration

**core-ecommerce → core-points (async, post-order):**
```
POST /v1/points/accrue   — 10 pts per USD of order subtotal
POST /v1/points/redeem   — if customer redeemed points at checkout
```

**bff-backoffice → core-points:**
```
GET /v1/stats                              — program KPIs for dashboard
GET /v1/customers/:id/profile-summary      — customer detail
GET /v1/points/:customerId/balance         — customer point balance
GET /v1/points/:customerId/transactions    — audit history
```

---

## Tech stack

- **Go 1.22** (stdlib `net/http`)
- **pgx/v5** + `pgxpool` for Postgres
- **Prometheus** client (custom registry)
- Structured JSON logging
- Schema auto-migration via `initDB()`

---

## Running locally

```bash
cp .env.example .env
# Edit DATABASE_URL

go run .
```

### Environment variables

| Variable | Description |
|---|---|
| `PORT` | HTTP listen port (default `3001`) |
| `DATABASE_URL` | PostgreSQL connection string |

---

## Part of loyalty-platform

See the [monorepo root](https://github.com/PabloJTechDev/loyalty-platform) for the full architecture, port map, and Docker Compose setup.
