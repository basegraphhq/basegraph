# Security Checklist

This document tracks security considerations, known issues, and mitigations for the Relay application.

## Authentication & Authorization

### Invite-Only Access

Relay uses an invite-only sign-in system. Users must have an accepted invitation to access the application.

**Flow:**
1. Admin creates invitation for email address
2. User receives invite link with unique token
3. User signs in via WorkOS (OAuth)
4. Backend validates email matches invitation
5. On match, invitation is marked as accepted and user is created/updated

### Known Issues & Mitigations

#### FIXED: Email Case Sensitivity
- **Issue:** Email lookups were case-sensitive, which could cause legitimate users to be blocked if WorkOS returned a differently-cased email than what was stored.
- **Fix:** All email comparisons now use `LOWER()` in SQL queries, and emails are normalized to lowercase on insert/update.
- **Files:** `relay/core/db/queries/users.sql`

#### DEFERRED: Invite Token Reusable After Partial Auth
- **Issue:** If user starts OAuth with valid invite token, `HandleCallback` upserts the user. If `Accept` then fails (e.g., email mismatch), the user exists in DB but invite is still "pending".
- **Risk:** Low. The invite token is still valid and could theoretically be used by someone else, but they'd need the token and the correct email.
- **Mitigation consideration:** Mark invite as "in_progress" before OAuth, or validate at `Accept()` that no user was created since validation.

#### DEFERRED: No Rate Limiting on Token Validation
- **Issue:** The `/invites/validate` endpoint has no rate limiting.
- **Risk:** Low. Tokens are 32 bytes of cryptographic randomness (256 bits entropy). Brute forcing is computationally infeasible.
- **Mitigation consideration:** Add rate limiting middleware if needed for defense-in-depth.

## Session Management

### Session Storage
- Sessions are stored in PostgreSQL with a 7-day expiry
- Session ID is a Snowflake ID (64-bit integer)
- WorkOS session ID is stored to enable full logout (clearing WorkOS session)

### Logout Flow
- Local logout: Deletes session from DB, clears cookie
- Full logout: Also redirects to WorkOS logout URL to clear their session

### Cookie Security
- `HttpOnly`: Yes (prevents XSS access)
- `Secure`: Yes in production (HTTPS only)
- `SameSite`: Lax (prevents CSRF for most cases)

## OAuth Security

### CSRF Protection
- State parameter is generated with 32 bytes of cryptographic randomness
- State is stored in HttpOnly cookie and validated on callback
- State cookie is cleared after use

### Code Exchange
- Authorization code is exchanged server-side with WorkOS
- Client never sees WorkOS access token (it's only used to extract session ID)

## Data Handling

### Email Normalization
- All emails are stored and compared in lowercase
- Prevents case-sensitivity issues and potential bypasses

### Sensitive Data
- Invite tokens are not logged
- Passwords are never stored (OAuth-only auth)
- WorkOS API key is stored in environment variables

## API Security

### Admin Endpoints
- Protected by API key (`X-Admin-API-Key` header or `Authorization: Bearer` token)
- Used for invite management only

### Authenticated Endpoints
- Require `X-Session-ID` header
- Session is validated on each request

## Known Attack Vectors to Monitor

1. **Invite token enumeration**: Tokens are random but validate endpoint could leak info. Currently returns different status codes for different states (expired vs used vs revoked).

2. **Email harvesting**: The validate endpoint returns the email for used/expired/revoked invites. This is intentional for UX but could leak invited emails.

3. **Session fixation**: New session is created on each OAuth callback. Old sessions are not invalidated automatically.

## Security Review Checklist for New Features

- [ ] Does it handle user input? Validate and sanitize.
- [ ] Does it access user data? Check authorization.
- [ ] Does it create sessions? Ensure proper expiry and HttpOnly cookies.
- [ ] Does it log anything? Ensure no sensitive data in logs.
- [ ] Does it make external requests? Validate URLs, use timeouts.
- [ ] Does it accept files? Validate type, size, content.
- [ ] Does it use SQL? Use parameterized queries (sqlc handles this).

## Incident Response

If a security issue is discovered:
1. Assess severity and scope
2. Document in this file with date
3. Fix with urgency appropriate to severity
4. Consider notifying affected users if data was exposed
5. Update this checklist with lessons learned

---

*Last updated: 2026-01-22*
