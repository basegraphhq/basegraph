# Production Deployment Checklist

## Database SSL Configuration

### PostgreSQL SSL Mode Setup
- [ ] **Configure SSL mode for Railway PostgreSQL**
  - Set `DATABASE_SSLMODE=require` in Railway environment variables
  - Railway uses self-signed certificates, so use `require` (not `verify-full`)
  - Never use `sslmode=disable` in production

- [ ] **Update default SSL mode in code** (optional improvement)
  - Consider updating `core/config/config.go` to default to `require` for production environments
  - Keep `disable` as default for local development only

- [ ] **Verify SSL connection**
  - Test database connection with SSL enabled
  - Confirm no SSL-related errors in logs
  - Verify encrypted connection in production

### Current Configuration
- Default SSL mode: `disable` (line 44 in `core/config/config.go`)
- Environment variable: `DATABASE_SSLMODE` (can override default)
- For Railway: Set `DATABASE_SSLMODE=require` in production

