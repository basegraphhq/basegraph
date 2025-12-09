# User, Organization & Workspace Onboarding

This document describes how users, organizations, and workspaces are created and managed in Relay, covering the onboarding flow, automatic workspace provisioning, and key design decisions.

## Overview

Relay uses a three-tier hierarchy:

```
User
  └── Organization (admin)
       └── Workspace (default, auto-created)
            ├── Integrations (GitLab, Linear, etc.)
            └── Repositories
```

**Key principles:**
- Users are identified by email (unique).
- Organizations have an admin user and a unique slug.
- Every organization automatically gets one default workspace on creation.
- Workspaces are org-scoped and contain integrations/repositories.

---

## User Onboarding

### User Sync (`POST /api/v1/auth/sync`)

Used by the dashboard after authentication (Better Auth session). This is the primary user entry point.

**What it does:**
1. Upserts user by email (insert if new, update if exists).
2. Returns user + list of organizations where user is admin.
3. Dashboard uses `has_organization` flag to decide next step:
   - If `true`: navigate to dashboard.
   - If `false`: prompt org creation.

**Request:**
```json
{
  "name": "Alice",
  "email": "alice@example.com",
  "avatar_url": "https://example.com/avatar.jpg"
}
```

**Response:**
```json
{
  "user": {
    "id": "123",
    "name": "Alice",
    "email": "alice@example.com",
    "avatar_url": "https://example.com/avatar.jpg",
    "created_at": "2025-12-09T10:00:00Z",
    "updated_at": "2025-12-09T10:00:00Z"
  },
  "organizations": [
    {
      "id": "456",
      "name": "Acme Corp",
      "slug": "acme-corp"
    }
  ],
  "has_organization": true
}
```

**Implementation:**
- Service: `UserService.Sync()`
- Store: `UserStore.Upsert()` (ON CONFLICT DO UPDATE on email)
- Queries admin-owned orgs via `OrganizationStore.ListByAdminUser()`

**ID generation:**
- Uses Snowflake IDs (`common/id.New()`) for globally unique, time-ordered 64-bit integers.

---

## Organization Creation

### Create Organization (`POST /api/v1/organizations`)

Creates an organization with a unique slug and automatically provisions a default workspace.

**Request:**
```json
{
  "name": "Acme Corp",
  "slug": "acme-corp",  // optional; generated from name if missing
  "admin_user_id": 123
}
```

**Response:**
```json
{
  "id": "456",
  "name": "Acme Corp",
  "slug": "acme-corp",
  "admin_user_id": 123,
  "created_at": "2025-12-09T10:00:00Z",
  "updated_at": "2025-12-09T10:00:00Z"
}
```

**What happens atomically (single transaction):**
1. Generate/validate org slug:
   - If provided, use it.
   - If missing, slugify org name.
   - If taken, append `-1`, `-2`, etc. (up to 20 attempts).
2. Create organization record.
3. Create default workspace:
   - Name: `<org name> workspace` (e.g., "Acme Corp workspace").
   - Slug: derived from org name (e.g., "acme-corp").
   - Org-scoped uniqueness: if slug taken, append `-1`, `-2`, etc.
   - Admin: same as org admin.
   - User: same as org admin (for now; multi-user support deferred).

**Implementation:**
- Service: `OrganizationService.Create()` wraps both in a transaction via `TxRunner`.
- Slug generation: `common.Slugify()` (lowercase, alphanumeric + hyphens).
- Workspace auto-creation: `createDefaultWorkspace()` called inside transaction.
- Transaction: Uses `core/db.WithTx()` for atomicity; rollback on any error.

**Slug collision handling:**
- Org slug: global uniqueness (checked via `OrganizationStore.GetBySlug`).
- Workspace slug: org-scoped uniqueness (checked via `WorkspaceStore.GetByOrgAndSlug`).

---

## Workspace Behavior

### Default Workspace

Every organization gets one default workspace on creation. This ensures:
- Integrations/repositories have a container from day one.
- No blocking "create workspace" step during onboarding.

**Workspace properties:**
- `name`: `<org name> workspace` (renameable later).
- `slug`: derived from org name with collision handling.
- `admin_user_id`: same as org admin.
- `user_id`: same as org admin (placeholder for future multi-user workspaces).
- `organization_id`: parent org.

### Future: Multiple Workspaces

The schema supports multiple workspaces per org, but UI/UX is deferred:
- Later: allow creating additional workspaces.
- Later: move integrations/repos between workspaces.
- For now: one default workspace per org, no multi-workspace UI.

### Workspace Scoping

Integrations and repositories are linked to a workspace:
- `integrations.workspace_id` → which workspace owns the integration.
- `repositories.workspace_id` → which workspace owns the repo.

This enables future org-level isolation (e.g., "production" vs "staging" workspaces).

---

## Data Flow

### First-Time User Journey

1. User signs in with Better Auth (OAuth or email/password).
2. Dashboard calls `POST /api/v1/auth/sync` with user info from session.
3. Relay upserts user, returns user + orgs.
4. If `has_organization == false`:
   - Dashboard prompts org creation form.
   - User submits org name (slug optional).
   - Dashboard calls `POST /api/v1/organizations`.
   - Relay creates org + default workspace in one transaction.
5. Dashboard redirects to main dashboard.

### Returning User Journey

1. User signs in.
2. Dashboard calls `POST /api/v1/auth/sync`.
3. Relay returns user + existing orgs.
4. Dashboard navigates directly to workspace view.

---

## Database Schema

### Users Table

```sql
CREATE TABLE users (
    id BIGINT PRIMARY KEY,                -- Snowflake ID
    name TEXT NOT NULL,
    email TEXT NOT NULL UNIQUE,           -- email is unique identifier
    avatar_url TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### Organizations Table

```sql
CREATE TABLE organizations (
    id BIGINT PRIMARY KEY,                -- Snowflake ID
    admin_user_id BIGINT NOT NULL REFERENCES users(id),
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,            -- global uniqueness
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    is_deleted BOOLEAN NOT NULL DEFAULT FALSE
);
```

### Workspaces Table

```sql
CREATE TABLE workspaces (
    id BIGINT PRIMARY KEY,                -- Snowflake ID
    admin_user_id BIGINT NOT NULL REFERENCES users(id),
    organization_id BIGINT NOT NULL REFERENCES organizations(id),
    user_id BIGINT NOT NULL REFERENCES users(id),  -- placeholder for multi-user
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
    CONSTRAINT unq_workspaces_org_slug UNIQUE (organization_id, slug)  -- org-scoped
);
```

**Key constraints:**
- Organization slug: globally unique.
- Workspace slug: unique within an organization.

---

## API Endpoints

### User Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/auth/sync` | Sync user from auth session; returns user + orgs |
| `POST` | `/api/v1/users` | Create user (rarely used; sync preferred) |

### Organization Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/organizations` | Create org + default workspace (atomic) |

### Workspace Endpoints

No public workspace creation endpoint yet. Workspaces are auto-created with orgs.

---

## Design Decisions

### Why Auto-Create Default Workspace?

**Problem:** Without workspaces, integrations/repos have nowhere to attach.

**Solution:** Auto-create one workspace per org on creation, eliminating an extra onboarding step.

**Trade-offs:**
- Simpler onboarding (no "create workspace" prompt).
- Schema supports multiple workspaces for future growth.
- Renameable later if user wants to customize.

**Inspiration:** Vercel's default project per team; sensible default, upgradeable later.

### Why Transactional Org + Workspace Creation?

**Problem:** If org creation succeeds but workspace creation fails, the org exists without a workspace—breaking assumptions downstream (e.g., integration setup).

**Solution:** Wrap both in a single transaction (`db.WithTx`). On any error, rollback both.

**Implementation:**
- `TxRunner` interface abstracts transaction logic.
- Service layer calls `TxRunner.WithTx()` and gets transactional stores.
- Slug checks, org create, workspace create—all in one atomic block.

**Benefits:**
- No orphaned orgs.
- Consistent state on failure.
- Easy to test (mock Tx runner).

### Why Slugs?

**Org slug:** Human-readable, unique identifier for URLs/routing (future: `relay.app/org/acme-corp`).

**Workspace slug:** Org-scoped, human-readable (future: `relay.app/org/acme-corp/workspace/production`).

**Collision handling:** Numeric suffix (`acme-1`, `acme-2`) ensures availability; simple, deterministic, no retries needed.

### Why User Sync Over Create?

**Problem:** User may sign in multiple times (e.g., refresh, new session). We don't want duplicate user errors.

**Solution:** Upsert on email; idempotent, returns existing user if already created.

**Dashboard flow:** Call sync on every authenticated page load; cheap, no side effects if user exists.

---

## Testing

### Service Layer Tests (Ginkgo)

`internal/service/organization_test.go` covers:
- Org creation with provided slug + default workspace.
- Slug generation from name.
- Org slug collision handling (numeric suffix).
- Workspace slug collision handling (org-scoped).
- Error propagation (slug check failures, workspace create failures).
- Transaction rollback (workspace create error aborts org creation).

**Mocks:**
- `mockTxRunner`: simulates transaction behavior.
- `mockOrganizationStore`, `mockWorkspaceStore`: stub DB calls.
- Call counters verify both org and workspace created exactly once.

---

## Future Enhancements

### Multi-Workspace UI

- Allow users to create additional workspaces within an org.
- Move integrations/repos between workspaces.
- Workspace-level permissions (who can access what).

### Workspace Invites

- Invite users to specific workspaces (not just org-level access).
- `workspace_members` table for many-to-many user-workspace relationships.

### Org Members

- Support multiple admins per org.
- Role-based access: `owner`, `admin`, `member`, `viewer`.

### Soft Deletes

- Both orgs and workspaces support `is_deleted` flag.
- Future: cascade soft-delete workspaces when org is deleted.

---

---

## Dashboard Integration

### Proxy-Based Route Protection

The dashboard uses a Next.js proxy file to enforce onboarding requirements server-side before pages render.

**Implementation:** `dashboard/proxy.ts` intercepts `/` and `/dashboard/*` routes and checks onboarding status via cookie:

```typescript
// dashboard/proxy.ts
export function proxy(request: NextRequest) {
  const { pathname } = request.nextUrl
  
  // Check dashboard routes (except onboarding page)
  if (pathname.startsWith('/dashboard') && pathname !== '/dashboard/onboarding') {
    const hasOrganization = request.cookies.get('relay-onboarding-complete')?.value === 'true'
    
    if (!hasOrganization) {
      // Redirect to onboarding BEFORE page renders
      return NextResponse.redirect(new URL('/dashboard/onboarding', request.url))
    }
  }
  
  // If on onboarding but already completed, redirect to dashboard
  if (pathname === '/dashboard/onboarding') {
    const hasOrganization = request.cookies.get('relay-onboarding-complete')?.value === 'true'
    
    if (hasOrganization) {
      return NextResponse.redirect(new URL('/dashboard', request.url))
    }
  }
  
  return NextResponse.next()
}
```

### Onboarding Status Cookie

The `relay-onboarding-complete` cookie tracks organization setup state.

**Cookie lifecycle:**
- Set by `/api/user/sync` based on `has_organization` from backend
- Set by `/api/organization/create` after successful organization creation
- Checked by proxy on every `/dashboard/*` request

**Cookie attributes:**
```typescript
{
  httpOnly: true,  // Not accessible from JavaScript
  secure: true,    // HTTPS-only in production
  sameSite: 'lax', // CSRF protection
  maxAge: 31536000, // 1 year
  path: '/',
}
```

### Request Flow

**First-time user:**
1. Sign in → Better Auth creates session
2. Navigate to `/dashboard`
3. Proxy checks cookie → not present → redirect to `/dashboard/onboarding`
4. Onboarding page renders, calls `/api/user/sync` (sets cookie based on backend response)
5. User creates organization → `/api/organization/create` sets cookie
6. Redirect to `/dashboard`
7. Proxy checks cookie → present → allow access

**Returning user:**
1. Sign in
2. Navigate to `/dashboard`
3. Proxy checks cookie → present → allow access immediately

### Authentication Provider Migration

The proxy architecture supports switching authentication providers with minimal code changes.

**Current implementation (Better Auth):**
- Onboarding state stored in `relay-onboarding-complete` cookie
- Cookie managed by dashboard API routes
- Proxy reads cookie to determine access

**WorkOS migration path:**
- Onboarding state stored in JWT claims via WorkOS user metadata
- JWT managed by WorkOS AuthKit
- Proxy reads JWT claim to determine access

**Migration involves:**
1. Configure WorkOS JWT template to include custom claims
2. Store `has_organization` in WorkOS user metadata (updated after org creation)
3. Update proxy to read from session JWT instead of cookie
4. Remove cookie management from API routes

The proxy structure and redirect logic remain unchanged.

---

## Summary

- **User sync** upserts by email; idempotent, returns orgs.
- **Org creation** auto-provisions a default workspace in a single transaction.
- **Workspaces** are org-scoped, slug-based, renameable; one per org for now.
- **Slugs** are human-readable, collision-safe (numeric suffix fallback).
- **Transactions** ensure atomic org + workspace creation; no orphaned state.
- **Proxy** enforces onboarding requirements server-side before page render.
- **Cookie-based** onboarding state tracking (current); JWT claims (planned with WorkOS).
- **Pre-beta simplicity:** Single default workspace per org; multi-workspace UX deferred.

For implementation details, see:
- Org service: `relay/internal/service/organization.go`
- User service: `relay/internal/service/user.go`
- Transaction runner: `relay/internal/service/txrunner.go`
- Dashboard proxy: `dashboard/proxy.ts`
- Tests: `relay/internal/service/organization_test.go`
