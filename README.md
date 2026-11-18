# Strata

**Strata** is an **open-source, self-hosted and local-first** project management system that lets companies and teams manage their projects, tasks and work processes from a single platform.

Teams can create organizations, manage their members and teams, collaborate on projects, track tasks and customize their workflows. Data is kept entirely on the organization's own infrastructure, so **data ownership, privacy and control** always stay with the user.

[![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-13%2B-336791?logo=postgresql&logoColor=white)](https://www.postgresql.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

---

## Status

Strata is currently **under development**. The core architecture and the authentication layer are ready and protected by tests:

- Authentication module — register, login, access/refresh token cycle
- PostgreSQL schema management (`users`, `refresh_tokens`)
- Server — routes mounted on the router, timeouts and graceful shutdown
- Test infrastructure — unit tests + integration tests against a real database

## Planned Features

| Area | Plan |
|---|---|
| Organizations | Create companies/workspaces, member & team management, role-based authorization |
| Projects & Tasks | Collaboration on projects, task tracking, customizable workflows |
| Work views | Kanban, list and calendar |
| Data ownership | Self-hosted, local-first; data stays on the organization's own infrastructure |

## API

| Endpoint | Body | Response |
|---|---|---|
| `POST /api/v1/auth/register` | `name`, `username`, `email`, `password` | `201` — `{"id","name","username","email"}` |
| `POST /api/v1/auth/login` | `email`, `password` | `200` — `{"accessToken","refreshToken"}` |
| `POST /api/v1/auth/refresh` | `refreshToken` | `200` — new `{"accessToken","refreshToken"}` |
| `GET /health` | — | `200` — `{"status":"ok"}` |

Example:

```bash
curl -X POST http://localhost:9090/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"name":"Kodcu","username":"kodcu_42","email":"kodcu@strata.dev","password":"sifre1234"}'
```

Errors return an `{"error":"..."}` body: `400` (invalid request/validation), `401` (wrong credentials or invalid refresh token), `409` (username/email already taken).

## Architecture

```mermaid
graph TD
    C[Client] --> R[chi Router]
    R --> H[HTTP Handlers<br/>internal/handlers]
    H --> V[Validation & Normalization<br/>internal/handlers/auth]
    H --> RP[Repository<br/>internal/repository]
    RP --> S[(PostgreSQL<br/>pgx v5 pool)]
```

| Technology | Purpose |
|---|---|
| [Go 1.27](https://go.dev) + [chi](https://github.com/go-chi/chi) | HTTP layer |
| [pgx v5](https://github.com/jackc/pgx) | PostgreSQL access (connection pool) |
| [golang-jwt/v5](https://github.com/golang-jwt/jwt) | HS256-signed access tokens |
| [bcrypt](https://pkg.go.dev/golang.org/x/crypto/bcrypt) | Password hashing |
| [godotenv](https://github.com/joho/godotenv) | `~/.strata/.env` configuration |

## Project Structure

```
Strata/
├── cmd/strata/            # Application entry point
├── internal/
│   ├── config/           # ~/.strata/.env setup and environment variables
│   ├── handlers/         # chi router and HTTP handlers
│   │   └── auth/         # Register / login / refresh endpoints
│   ├── models/           # Request-response models, JWT claims
│   ├── repository/       # PostgreSQL queries
│   └── store/            # Database connection and schema
└── StrataBrain/          # (Personal) project notes
```

## Security Design

The authentication layer implements its security decisions explicitly and documents them:

- **Passwords** are never stored in plaintext; they are hashed with bcrypt. bcrypt's 72-byte input limit is enforced at the validation layer with an explicit user-facing error.
- **Access tokens** are short-lived (15 minutes) and carry only the user id + username.
- **Refresh tokens** are 64-character random hex values; only their **SHA-256 hash** is stored in the database. A database leak cannot enable token replay.
- Refresh tokens are **single-use**: each refresh atomically deletes the old token (`DELETE ... RETURNING`) and issues a new one. Two concurrent requests cannot use the same token.
- **Usernames** are 3–20 characters, restricted to lowercase letters/digits/underscores, and reserved names (`admin`, `root`, `api`, ...) cannot be taken.
- Login returns the same error for a wrong email or a wrong password — which part of the credentials was incorrect is never disclosed.

## Configuration

On first run the application automatically creates the `~/.strata/.env` file:

```env
JWT_SECRET=<randomly generated 32-byte signing key>
DB_URL=   # must be filled in
```

- `JWT_SECRET` is generated automatically and written with `0600` permissions; an existing file is never overwritten.
- Fill `DB_URL` with your PostgreSQL connection URL, e.g. `postgres://user:pass@localhost:5432/strata`.

Start the server:

```bash
go run ./cmd/strata              # default port: 9090
go run ./cmd/strata -port 8080   # or any port you like
```

The schema (`users`, `refresh_tokens`) is created automatically at startup; it is based on `CREATE TABLE IF NOT EXISTS`, so re-running is safe. `Ctrl+C` / `SIGTERM` lets in-flight requests finish and shuts the server down cleanly.

## Development

Clone the project and fetch the dependencies:

```bash
git clone https://github.com/coderianx/Strata.git
cd Strata
go mod download
```

### Testing

```bash
# Unit tests (no database required, fast)
go test ./...

# With race detection
go test -race ./...

# Coverage report
go test -cover ./...
```

Integration tests require a real PostgreSQL instance and **truncate** tables — always point them at a disposable test database:

```bash
TEST_DATABASE_URL=postgres://user:pass@localhost:5432/strata_test \
    go test -tags=integration ./...
```

## Contributing

Strata is an open-source project. To contribute:

1. Open an issue or pick up an existing one.
2. Fork and create a feature branch.
3. Make sure your changes ship with unit (and, where needed, integration) tests: `go test -race ./...`
4. Open a pull request.

## License

This project is licensed under the [MIT License](LICENSE).
