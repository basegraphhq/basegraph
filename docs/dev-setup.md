# Developer Setup

Local development environment setup for the Relay monorepo.

---

## Prerequisites

- **Git**
- **Docker + Docker Compose v2**
- **Go** (see `relay/go.mod` for version)
- **Node.js 20+** (22 LTS recommended)
- **Bun** (package manager for dashboard)

---

## IDE Configuration (VS Code)

We commit shared VS Code configurations to keep the team consistent:

| File | Committed | Purpose |
|------|-----------|---------|
| `.vscode/launch.json` | Yes | Debug configurations for all services |
| `.vscode/tasks.json` | Yes | Shared build/test tasks |
| `.vscode/extensions.json` | Yes | Recommended extensions |
| `.vscode/settings.json` | No | Personal settings (fonts, themes, etc.) |

`settings.json` is gitignored—use it for personal preferences without affecting others.

---

## Relay Setup

### Install Tools

```bash
cd relay
make install-tools
```

Installs: sqlc, goose, ginkgo, gofumpt, golangci-lint

### Environment Setup

```bash
# Option 1: Generate minimal .env with defaults
make .env

# Option 2: Copy example and customize
cp .env.example .env
```

### Start Dev Infrastructure

```bash
make dev-db       # Starts Postgres + Redis via Docker
make migrate-up   # Apply database migrations
```

### Run Services

```bash
make run-server   # API server (port 8080)
make run-worker   # Background worker (separate terminal)
```

### Testing

```bash
make test              # Run all tests
make test-unit         # Unit tests only
make test-bdd          # Ginkgo BDD tests
make test-integration  # Integration tests (requires dev-db)
```

### Code Quality

```bash
make format   # Format with gofumpt
make lint     # Run golangci-lint
```

### Database Commands

```bash
make sqlc-generate           # Generate Go code from SQL
make migrate-create NAME=foo # Create new migration
make migrate-down            # Rollback last migration
```

### Cleanup

```bash
make dev-down   # Stop Postgres + Redis
make clean      # Remove compiled binaries
```

---

## Dashboard Setup

### Install Dependencies

```bash
cd dashboard
bun install
```

### Run Dev Server

```bash
bun run dev   # http://localhost:3000
```

### Build & Production

```bash
bun run build   # Production build
bun run start   # Start production server
```

### Linting

```bash
bun run lint
```

---

## Running Full Stack Locally

1. **Start infrastructure:**
   ```bash
   cd relay
   make dev-db
   make migrate-up
   ```

2. **Start Relay (two terminals):**
   ```bash
   make run-server   # Terminal 1
   make run-worker   # Terminal 2
   ```

3. **Start Dashboard:**
   ```bash
   cd dashboard
   bun run dev
   ```

4. **Open:** http://localhost:3000

---

## Environment Variables

### Relay (relay/.env)

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_URL` | `postgres://postgres:postgres@localhost:5432/relay?sslmode=disable` | PostgreSQL connection |
| `REDIS_URL` | `redis://localhost:6379/0` | Redis connection |
| `PORT` | `8080` | Server port |
| `ENV` | `development` | Environment |
| `DASHBOARD_URL` | `http://localhost:3000` | Dashboard URL |

**Optional:**

| Variable | Description |
|----------|-------------|
| `WORKOS_API_KEY` | WorkOS SSO (not needed for local dev) |
| `OPENAI_API_KEY` | Required if `LLM_ENABLED=true` |
| `LLM_ENABLED` | Set `false` to bypass LLM features |

See `relay/.env.example` for the full list.

### Dashboard

No `.env.example` currently defined. The dashboard connects to Relay at `http://localhost:8080` by default.

---

## Debug Configurations

The `.vscode/launch.json` includes:

- **Relay Server** — `relay/cmd/server`
- **Relay Worker** — `relay/cmd/worker`
- **Codegraph CLI** — `codegraph/cmd/codegraph`
- **Dashboard** — Next.js with Bun
- **Relay (Server + Worker)** — compound config for both
