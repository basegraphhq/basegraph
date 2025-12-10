# Production Deployment Security Checklist

## Database SSL Configuration

### PostgreSQL SSL Mode Setup
- [ ] **Configure SSL mode for Railway PostgreSQL**
  - Set `DATABASE_URL` with `sslmode=require` in Railway environment variables
  - Railway uses self-signed certificates, so use `require` (not `verify-full`)
  - Never use `sslmode=disable` in production
  - Example: `postgres://user:password@host:port/dbname?sslmode=require`

- [ ] **Verify SSL connection**
  - Test database connection with SSL enabled
  - Confirm no SSL-related errors in logs
  - Verify encrypted connection in production

### Current Configuration
- Environment variable: `DATABASE_URL` (single connection string)
- Default: `postgres://postgres:postgres@localhost:5432/basegraph?sslmode=disable` (development only)
- For Railway: Set `DATABASE_URL` with `sslmode=require` in production

