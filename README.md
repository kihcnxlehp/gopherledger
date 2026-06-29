# Gopherledger

A loyalty system backend for an e-commerce platform, written in Go.  
Users register, upload order numbers, and earn reward points that can be redeemed for future purchases.

![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)
![License](https://img.shields.io/badge/License-MIT-green.svg)

---

## Features

- 🔐 **Authentication** — Registration and login with JWT tokens and bcrypt password hashing
- 🛒 **Order Upload** — Order number validation using the Luhn algorithm
- ⚙️ **Automatic Points Accrual** — Background worker with concurrent order processing
- 💳 **Points Redemption** — Balance checking and transaction history
- 📊 **Statistics Export** — Report generation to a text file
-  **Graceful Shutdown** — Correct server termination on SIGINT/SIGTERM signals
- ⚛️ **Atomic Operations** — `sync/atomic` used for ID generation

---

## Tech Stack

| Technology | Purpose |
|------------|---------|
| **Go 1.22+** | Programming language |
| **JWT** (`golang-jwt/jwt/v5`) | Stateless authentication |
| **bcrypt** (`golang.org/x/crypto/bcrypt`) | Secure password hashing |
| **errgroup** (`golang.org/x/sync/errgroup`) | Concurrent order processing |
| **YAML** (`gopkg.in/yaml.v3`) | Application configuration |
| **In-memory store** | Thread-safe data storage |

---

## Architecture

The project is built following **Clean Architecture** principles with clear layer separation:

```
┌─────────────────────────────────────────────────────────┐
│                     HTTP Layer                           │
│  router → middleware (Auth, Logging, Recover)            │
│         → handler                                        │
└───────────────────────┬─────────────────────────────────┘
                        │ Service interface
┌───────────────────────▼─────────────────────────────────┐
│                   Business Layer                         │
│  service (business logic, Luhn validation, worker)       │
└───────────────────────┬─────────────────────────────────┘
                        │ Repository interface
┌───────────────────────▼─────────────────────────────────
│                   Data Layer                             │
│  store (in-memory storage with mutexes and atomic)       │
└─────────────────────────────────────────────────────────┘
```

**Key Design Decisions:**
- Dependencies point **inward** — `service` and `handler` depend on interfaces, not concrete implementations
- Sentinel errors are centralized in the `domain` package
- Concurrent data access is protected by `sync.RWMutex` + `sync/atomic`
- The accrual worker uses `errgroup` with concurrency limits

---

## Project Structure

```
gopherledger/
├── cmd/server/
│   └── main.go            # Entry point, graceful shutdown
├── internal/
│   ├── auth/              # JWT token generation and validation
│   ├── config/            # YAML configuration loading
│   ├── domain/            # Business models and sentinel errors
│   ├── handler/           # HTTP handlers
│   ├── middleware/        # Auth, Logging, Recover
│   ├── router/            # Route registration
│   ├── service/           # Business logic and accrual worker
│   └── store/             # In-memory repository
├── config.example.yaml    # Configuration template
├── go.mod
├── go.sum
└── README.md
```

---

## API Reference

### Public Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/user/register` | Register a new user |
| `POST` | `/api/user/login` | Log in to the system |

### Protected Endpoints (require `Authorization` header)

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/user/orders` | Upload an order number |
| `GET`  | `/api/user/orders` | Get user's order list |
| `GET`  | `/api/user/balance` | Get current balance |
| `POST` | `/api/user/balance/withdraw` | Redeem points |
| `GET`  | `/api/user/withdrawals` | Get redemption history |
| `POST` | `/api/stats/export` | Export statistics to a file |

---

## Quick Start

### 1. Clone the repository

```bash
git clone https://github.com/kihcnxlehp/gopherledger.git
cd gopherledger
```

### 2. Configure the application

```bash
cp config.example.yaml config.yaml
```

Edit `config.yaml` to suit your needs:

```yaml
server_host: "localhost"
server_port: 8080
log_level: "info"
accrual_interval_seconds: 3
worker_concurrency: 5
```

### 3. Run the server

```bash
go run ./cmd/server
```

The server will start at `http://localhost:8080`.

---

## Usage Examples

### Registration

```bash
curl -X POST http://localhost:8080/api/user/register \
  -H "Content-Type: application/json" \
  -d '{"login":"alice","password":"secret123"}'
```

Response: `200 OK`, token in the `Authorization` header.

### Login

```bash
curl -X POST http://localhost:8080/api/user/login \
  -H "Content-Type: application/json" \
  -d '{"login":"alice","password":"secret123"}'
```

### Upload an Order

```bash
TOKEN="<your_token>"

curl -X POST http://localhost:8080/api/user/orders \
  -H "Authorization: $TOKEN" \
  -d "4111111111111111"
```

Response: `202 Accepted` — order accepted for processing.

### Check Balance

```bash
curl -X GET http://localhost:8080/api/user/balance \
  -H "Authorization: $TOKEN"
```

Response:
```json
{
  "current": 150.5,
  "withdrawn": 50
}
```

### Redeem Points

```bash
curl -X POST http://localhost:8080/api/user/balance/withdraw \
  -H "Authorization: $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"order":"4111111111111111","sum":30}'
```

---

## Testing

```bash
# Run all tests
go test ./...

# Run with race detector (mandatory for store)
go test -race ./...

# Run with coverage
go test -cover ./...

# Generate HTML coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

**Package Coverage:**

| Package | Coverage |
|---------|----------|
| `store` | ~92%     |
| `handler` | ~56%     |
| `service` | ~49%     |
| `middleware` | ~71%     |
| `auth` | ~75%     |

---

## Concurrency & Security

### Race Condition Prevention
- Each data type in `store` is protected by a dedicated `sync.RWMutex`
- ID generation is implemented via `sync/atomic.AddInt64` — no nested locks
- The accrual worker uses an atomic `tryMarkProcessing` operation to prevent duplicate order processing (TOCTOU protection)

### Security
- Passwords are hashed using **bcrypt** (not SHA-256)
- Tokens are **JWT with TTL**, not stored on the server (stateless)
- Defense-in-depth: every handler checks for userID in the context, even if the middleware has already done so

---

## License

MIT License. See the [LICENSE](LICENSE) file for details.