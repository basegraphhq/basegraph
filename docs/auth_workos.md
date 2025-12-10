# WorkOS AuthKit Integration

This document explains how authentication works in Basegraph using WorkOS AuthKit.

## Table of Contents

1. [What is WorkOS?](#what-is-workos)
2. [Why WorkOS?](#why-workos)
3. [Architecture Overview](#architecture-overview)
4. [Authentication Flow](#authentication-flow)
5. [Implementation Details](#implementation-details)
6. [Session Management](#session-management)
7. [Environment Variables](#environment-variables)
8. [WorkOS Dashboard Setup](#workos-dashboard-setup)
9. [Local Development](#local-development)
10. [Security Considerations](#security-considerations)
11. [Troubleshooting](#troubleshooting)

---

## What is WorkOS?

[WorkOS](https://workos.com) is an identity platform that provides enterprise-ready authentication features:

- **AuthKit**: A complete authentication solution with login UI, session management, and user management
- **SSO (Single Sign-On)**: Connect to enterprise identity providers (Okta, Azure AD, Google Workspace, JumpCloud, etc.)
- **Directory Sync (SCIM)**: Automatically sync users and groups from identity providers
- **Audit Logs**: Track authentication events for compliance

### Key Concepts

| Term | Description |
|------|-------------|
| **AuthKit** | WorkOS's hosted authentication UI. Users are redirected here to sign in. |
| **Client ID** | Public identifier for your application (safe to expose) |
| **API Key** | Secret key for server-to-server calls (never expose to browser) |
| **Redirect URI** | URL WorkOS sends users back to after authentication |
| **Authorization Code** | One-time code exchanged for user information |
| **State Parameter** | Random string to prevent CSRF attacks |

---

## Why WorkOS?

We chose WorkOS over alternatives (Auth0, Clerk, Firebase Auth, self-hosted) for these reasons:

### 1. Enterprise SSO First-Class Support

Our target customers are engineering teams at companies using enterprise identity providers. WorkOS makes connecting SSO providers (JumpCloud, Okta, Azure AD) straightforward, with automatic user provisioning.

### 2. Clean SDK and API

The WorkOS Go SDK is well-designed and doesn't require heavy abstractions:

```go
// Exchange authorization code for user
authResponse, err := usermanagement.AuthenticateWithCode(ctx, usermanagement.AuthenticateWithCodeOpts{
    ClientID: clientID,
    Code:     code,
})
user := authResponse.User
```

### 3. No Vendor Lock-in for User Data

WorkOS doesn't force you to use their user database. We maintain our own `users` table and link it via `workos_id`. If we ever migrate away, we keep all user data.

### 4. Pricing Model

WorkOS charges per SSO connection, not per user. This works well for B2B SaaS where you might have a few large customers with many users each.

### Alternatives Considered

| Alternative | Why Not |
|-------------|---------|
| **Auth0** | Complex, expensive at scale, heavy SDK |
| **Clerk** | Great DX but more consumer-focused, less enterprise SSO flexibility |
| **Firebase Auth** | Limited SSO support, Google ecosystem lock-in |
| **Self-hosted (Keycloak)** | Operational overhead, we're a small team |
| **Better Auth** | We initially used this, but it's more suited for simpler auth flows. Migrating to WorkOS for enterprise SSO. |

---

## Architecture Overview

### The Challenge

Our infrastructure has a constraint:

- **Dashboard** (Next.js): `basegraph.app` — public-facing
- **Relay** (Go): `basegraph.railway.internal` — internal Railway network, NOT exposed to internet

The browser cannot directly call Relay. All API calls must go through Dashboard's server.

### Our Solution: Dashboard Proxies Auth

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                                                                                 │
│  Browser                    Dashboard                   Relay         WorkOS    │
│     │                      (basegraph.app)           (internal)                 │
│     │                           │                        │              │       │
│     │ 1. Click "Sign In"        │                        │              │       │
│     │──────────────────────────▶│                        │              │       │
│     │                           │                        │              │       │
│     │                           │ 2. GET /auth/url       │              │       │
│     │                           │───────────────────────▶│              │       │
│     │                           │◀── {url, state} ───────│              │       │
│     │                           │                        │              │       │
│     │◀── 3. Set state cookie ───│                        │              │       │
│     │    + redirect to WorkOS   │                        │              │       │
│     │                           │                        │              │       │
│     │ 4. User authenticates ────────────────────────────────────────────▶       │
│     │                           │                        │              │       │
│     │◀── 5. Redirect with code ─────────────────────────────────────────│       │
│     │    to /api/auth/callback  │                        │              │       │
│     │                           │                        │              │       │
│     │──────────────────────────▶│ 6. POST /auth/exchange │              │       │
│     │                           │    {code}              │              │       │
│     │                           │───────────────────────▶│              │       │
│     │                           │                        │──── 7. ─────▶│       │
│     │                           │                        │   Verify     │       │
│     │                           │                        │◀── user ─────│       │
│     │                           │◀── {user, session} ────│              │       │
│     │                           │                        │              │       │
│     │◀── 8. Set session cookie ─│                        │              │       │
│     │    + redirect to dashboard│                        │              │       │
│     │                           │                        │              │       │
└─────────────────────────────────────────────────────────────────────────────────┘
```

### Why This Architecture?

| Alternative | Problem |
|-------------|---------|
| **Relay handles auth directly** | Relay is internal-only, browser can't reach it |
| **Expose Relay's `/auth/*` publicly** | Security risk, infrastructure complexity |
| **Dashboard calls WorkOS directly** | Duplicates auth logic, Relay wouldn't know about users |

Our approach:
- Dashboard handles browser interaction (cookies, redirects)
- Relay handles business logic (user upsert, session creation)
- WorkOS handles identity verification

---

## Authentication Flow

### Step-by-Step Breakdown

#### 1. User Clicks "Sign In"

```tsx
// dashboard/components/login-button.tsx
const handleLogin = () => {
  window.location.href = getLoginUrl()  // "/api/auth/login"
}
```

#### 2. Dashboard Gets Auth URL from Relay

```typescript
// dashboard/app/api/auth/login/route.ts
export async function GET() {
  // Call Relay to get WorkOS authorization URL
  const res = await fetch(`${RELAY_API_URL}/auth/url`)
  const data = await res.json()
  // data = { authorization_url: "https://auth.workos.com/...", state: "abc123..." }
  
  // Store state in cookie for CSRF protection
  cookies().set('relay_oauth_state', data.state, { ... })
  
  // Redirect user to WorkOS
  return NextResponse.redirect(data.authorization_url)
}
```

#### 3. Relay Generates Authorization URL

```go
// relay/internal/http/handler/auth.go
func (h *AuthHandler) GetAuthURL(c *gin.Context) {
    state, _ := generateState()  // Random 32-byte string
    
    authURL, _ := usermanagement.GetAuthorizationURL(usermanagement.GetAuthorizationURLOpts{
        ClientID:    h.cfg.ClientID,
        RedirectURI: h.cfg.RedirectURI,  // "https://basegraph.app/api/auth/callback"
        State:       state,
        Provider:    "authkit",
    })
    
    c.JSON(http.StatusOK, GetAuthURLResponse{
        AuthorizationURL: authURL.String(),
        State:            state,
    })
}
```

#### 4. User Authenticates at WorkOS

WorkOS presents the AuthKit login UI. User can:
- Sign in with email/password
- Sign in with Google
- Sign in with enterprise SSO (if configured for their domain)

#### 5. WorkOS Redirects Back with Code

After successful authentication, WorkOS redirects to:
```
https://basegraph.app/api/auth/callback?code=abc123&state=xyz789
```

#### 6. Dashboard Validates State and Exchanges Code

```typescript
// dashboard/app/api/auth/callback/route.ts
export async function GET(request: NextRequest) {
  const code = searchParams.get('code')
  const state = searchParams.get('state')
  
  // Validate state matches what we stored (CSRF protection)
  const storedState = cookies().get('relay_oauth_state')?.value
  if (state !== storedState) {
    return NextResponse.redirect('/?auth_error=invalid_state')
  }
  
  // Exchange code for user/session via Relay
  const res = await fetch(`${RELAY_API_URL}/auth/exchange`, {
    method: 'POST',
    body: JSON.stringify({ code }),
  })
  const data = await res.json()
  // data = { user: {...}, session_id: "123456789", expires_in: 168 }
  
  // Set session cookie
  cookies().set('relay_session', data.session_id, { ... })
  
  return NextResponse.redirect('/dashboard')
}
```

#### 7. Relay Exchanges Code with WorkOS

```go
// relay/internal/service/auth.go
func (s *authService) HandleCallback(ctx context.Context, code string) (*model.User, *model.Session, error) {
    // Exchange code with WorkOS
    authResponse, err := usermanagement.AuthenticateWithCode(ctx, usermanagement.AuthenticateWithCodeOpts{
        ClientID: s.cfg.ClientID,
        Code:     code,
    })
    
    workosUser := authResponse.User
    
    // Upsert user in our database (create if new, update if existing)
    user := &model.User{
        ID:        id.New(),
        Name:      buildUserName(workosUser),
        Email:     workosUser.Email,
        AvatarURL: &workosUser.ProfilePictureURL,
        WorkOSID:  &workosUser.ID,
    }
    s.userStore.UpsertByWorkOSID(ctx, user)
    
    // Create session
    session := &model.Session{
        ID:        id.New(),
        UserID:    user.ID,
        ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
    }
    s.sessionStore.Create(ctx, session)
    
    return user, session, nil
}
```

#### 8. User is Authenticated

The `relay_session` cookie is now set on `basegraph.app`. Subsequent requests include this cookie, and the proxy/middleware validates it.

---

## Implementation Details

### Files Overview

#### Relay (Go)

| File | Purpose |
|------|---------|
| `core/config/config.go` | WorkOS configuration (API key, client ID, redirect URI) |
| `internal/http/handler/auth.go` | HTTP handlers for auth endpoints |
| `internal/http/router/auth.go` | Route definitions |
| `internal/service/auth.go` | Business logic (WorkOS SDK calls, user/session management) |
| `internal/store/user.go` | User database operations |
| `internal/store/session.go` | Session database operations |
| `internal/model/user.go` | User domain model |
| `internal/model/session.go` | Session domain model |
| `core/db/queries/users.sql` | SQL queries for users |
| `core/db/queries/sessions.sql` | SQL queries for sessions |

#### Dashboard (Next.js)

| File | Purpose |
|------|---------|
| `app/api/auth/login/route.ts` | Initiates login flow |
| `app/api/auth/callback/route.ts` | Handles WorkOS callback |
| `app/api/auth/me/route.ts` | Returns current user |
| `app/api/auth/logout/route.ts` | Logs out user |
| `lib/auth.ts` | Auth utilities (getSession, signOut, getLoginUrl) |
| `hooks/use-session.ts` | React hook for session state |
| `proxy.ts` | Next.js middleware for route protection |
| `components/login-button.tsx` | Sign in button |
| `components/logout-button.tsx` | Sign out button |

### Relay Auth Endpoints

| Endpoint | Method | Purpose | Used By |
|----------|--------|---------|---------|
| `/auth/url` | GET | Get WorkOS authorization URL + state | Dashboard login route |
| `/auth/exchange` | POST | Exchange code for user/session | Dashboard callback route |
| `/auth/validate` | GET | Validate session, return user | Dashboard me route, proxy |
| `/auth/logout-session` | POST | Delete session | Dashboard logout route |

### Database Schema

```sql
-- Users table
CREATE TABLE users (
    id          BIGINT PRIMARY KEY,        -- Snowflake ID
    name        TEXT NOT NULL,
    email       TEXT NOT NULL UNIQUE,
    avatar_url  TEXT,
    workos_id   TEXT UNIQUE,               -- Links to WorkOS user
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);

-- Sessions table
CREATE TABLE sessions (
    id          BIGINT PRIMARY KEY,        -- Snowflake ID (also the session token)
    user_id     BIGINT NOT NULL REFERENCES users(id),
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    expires_at  TIMESTAMPTZ NOT NULL       -- 7 days from creation
);
```

---

## Session Management

### How Sessions Work

1. **Session ID = Token**: We use Snowflake IDs as session tokens. They're:
   - Unique across all instances
   - Time-ordered (newer sessions have higher IDs)
   - Not guessable (64-bit with randomness)

2. **Cookie Storage**: The session ID is stored in an HTTP-only cookie:
   ```
   relay_session=1234567890123456789
   ```

3. **Validation**: On each request, the session ID is validated against the database:
   ```sql
   SELECT * FROM sessions WHERE id = $1 AND expires_at > NOW();
   ```

4. **Expiration**: Sessions expire after 7 days. Expired sessions are periodically cleaned up.

### Session Cookie Properties

```typescript
cookies().set('relay_session', sessionId, {
  httpOnly: true,      // Not accessible via JavaScript (XSS protection)
  secure: true,        // Only sent over HTTPS (in production)
  sameSite: 'lax',     // Sent on top-level navigations (CSRF protection)
  maxAge: 604800,      // 7 days in seconds
  path: '/',           // Available on all paths
})
```

### Why Not JWTs?

| JWTs | Our Approach (Session IDs) |
|------|---------------------------|
| Stateless - can't revoke | Can revoke instantly by deleting from DB |
| Contains user data (can leak) | Just an opaque ID |
| Larger payload | Minimal cookie size |
| Complex signature verification | Simple DB lookup |
| Good for distributed systems | Good for our scale |

We might add JWTs later for API access tokens, but for browser sessions, database-backed sessions are simpler and more secure.

---

## Environment Variables

### Relay

```bash
# WorkOS Configuration
WORKOS_API_KEY=sk_live_...          # From WorkOS dashboard (keep secret!)
WORKOS_CLIENT_ID=client_...          # From WorkOS dashboard (can be public)
WORKOS_REDIRECT_URI=https://basegraph.app/api/auth/callback

# Dashboard URL (for redirects after auth)
DASHBOARD_URL=https://basegraph.app
```

### Dashboard

```bash
# Internal Relay URL (Railway private network)
RELAY_API_URL=http://basegraph.railway.internal:8080

# Public URL (for redirects)
NEXT_PUBLIC_APP_URL=https://basegraph.app
```

---

## WorkOS Dashboard Setup

### 1. Create a WorkOS Account

1. Go to [workos.com](https://workos.com) and sign up
2. Create a new project (e.g., "Basegraph")

### 2. Get Credentials

1. Go to **API Keys** in the sidebar
2. Copy the **API Key** (starts with `sk_`)
3. Go to **Configuration** > **AuthKit**
4. Copy the **Client ID** (starts with `client_`)

### 3. Configure Redirect URI

1. Go to **Configuration** > **Redirects**
2. Add your redirect URI: `https://basegraph.app/api/auth/callback`
3. For local development, also add: `http://localhost:3000/api/auth/callback`

### 4. Configure AuthKit

1. Go to **Configuration** > **AuthKit**
2. Enable **AuthKit** toggle
3. Configure authentication methods:
   - **Email + Password**: Enable for email/password login
   - **Google OAuth**: Enable for "Sign in with Google"
   - **Magic Link**: Optional, for passwordless email login

### 5. Configure SSO (Enterprise)

For each enterprise customer:

1. Go to **Organizations** > Create organization
2. Add the customer's domain (e.g., `acme.com`)
3. Go to **SSO** > Add connection
4. Select their identity provider (Okta, Azure AD, JumpCloud, etc.)
5. Follow the setup wizard to exchange metadata

Once configured, users with `@acme.com` emails will automatically be redirected to their SSO provider.

---

## Local Development

### Setup

1. **Get WorkOS credentials** from the dashboard

2. **Add local redirect URI** in WorkOS:
   ```
   http://localhost:3000/api/auth/callback
   ```

3. **Create `.env` files**:

   ```bash
   # relay/.env
   WORKOS_API_KEY=sk_test_...
   WORKOS_CLIENT_ID=client_...
   WORKOS_REDIRECT_URI=http://localhost:3000/api/auth/callback
   DASHBOARD_URL=http://localhost:3000
   ```

   ```bash
   # dashboard/.env
   RELAY_API_URL=http://localhost:8080
   NEXT_PUBLIC_APP_URL=http://localhost:3000
   ```

4. **Run services**:
   ```bash
   # Terminal 1: Relay
   cd relay && make run
   
   # Terminal 2: Dashboard
   cd dashboard && bun dev
   ```

### Testing the Flow

1. Open `http://localhost:3000`
2. Click "Sign In"
3. You'll be redirected to WorkOS AuthKit
4. Sign in with email/password or Google
5. You'll be redirected back to `http://localhost:3000/dashboard`

### Common Issues

| Issue | Solution |
|-------|----------|
| "Invalid redirect URI" | Add `http://localhost:3000/api/auth/callback` to WorkOS |
| "Invalid state" | Clear cookies and try again |
| Session not persisting | Check cookie settings (SameSite, Secure) |
| Relay connection refused | Ensure Relay is running on port 8080 |

---

## Security Considerations

### 1. State Parameter (CSRF Protection)

We generate a random state for each login attempt:

```go
func generateState() (string, error) {
    b := make([]byte, 32)
    rand.Read(b)
    return base64.URLEncoding.EncodeToString(b), nil
}
```

This prevents attackers from initiating auth flows and tricking users into linking attacker-controlled accounts.

### 2. HTTP-Only Cookies

Session cookies are HTTP-only, preventing JavaScript access:

```typescript
httpOnly: true  // Cannot be read by document.cookie
```

This mitigates XSS attacks - even if an attacker injects JavaScript, they can't steal the session.

### 3. Secure Cookies in Production

```typescript
secure: process.env.NODE_ENV === 'production'
```

In production, cookies are only sent over HTTPS, preventing interception.

### 4. SameSite Cookies

```typescript
sameSite: 'lax'
```

Cookies are only sent on top-level navigations, not on cross-site requests. This prevents CSRF attacks while still allowing normal navigation.

### 5. Session Expiration

Sessions expire after 7 days:

```go
ExpiresAt: time.Now().Add(7 * 24 * time.Hour)
```

Expired sessions are cleaned up periodically.

### 6. No Sensitive Data in Cookies

We only store a session ID in the cookie, not user data:

```
relay_session=1234567890123456789
```

User data is fetched from the database on each request.

### 7. WorkOS API Key Security

The API key is only used server-side (in Relay). It's never exposed to the browser.

---

## Troubleshooting

### "Not authenticated" after login

1. Check if `relay_session` cookie exists in browser dev tools
2. Verify the session exists in the database:
   ```sql
   SELECT * FROM sessions WHERE id = <session_id>;
   ```
3. Check if session is expired (`expires_at < NOW()`)

### "Invalid state" error

1. Clear all cookies for the domain
2. Try logging in again
3. If persists, check that cookies are being set correctly (dev tools > Application > Cookies)

### WorkOS returns error

1. Check Relay logs for the specific error
2. Verify WorkOS credentials are correct
3. Ensure redirect URI matches exactly (including trailing slashes)

### Session doesn't persist across refreshes

1. Check cookie `SameSite` and `Secure` settings
2. In development, ensure `Secure: false` (HTTPS not required)
3. Check browser privacy settings (some block third-party cookies)

### "User not found" after successful auth

1. Check if user was created in the database:
   ```sql
   SELECT * FROM users WHERE workos_id = '<workos_user_id>';
   ```
2. Check Relay logs for database errors
3. Verify database connection

---

## Future Improvements

1. **Refresh Tokens**: Implement token refresh to extend sessions without re-authentication
2. **Multi-Session Support**: Allow users to see and revoke active sessions
3. **Remember Me**: Option for longer session duration
4. **MFA Support**: Leverage WorkOS MFA capabilities
5. **Organization-Level SSO**: Route users to their org's SSO automatically based on email domain

---

## References

- [WorkOS Documentation](https://workos.com/docs)
- [WorkOS Go SDK](https://github.com/workos/workos-go)
- [AuthKit Guide](https://workos.com/docs/user-management/authkit)
- [SSO Integration Guide](https://workos.com/docs/sso)
