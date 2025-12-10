# WorkOS Organization Sync (Future)

Notes for implementing enterprise SSO support when ready.

---

## Context

- First potential customer uses JumpCloud
- Need to future-proof DB design for proper B2B SaaS
- Current auth flow works (WorkOS AuthKit), but no org-level SSO yet

---

## Schema Changes Needed

### 1. Link Organizations to WorkOS

```sql
ALTER TABLE organizations ADD COLUMN workos_organization_id TEXT UNIQUE;
```

This enables SSO connections per organization. Only set for orgs that need enterprise SSO.

### 2. Membership Table

Replace the `admin_user_id` single-owner approach with proper membership:

```sql
CREATE TABLE organization_memberships (
    id              BIGINT PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organizations(id),
    user_id         BIGINT NOT NULL REFERENCES users(id),
    role            TEXT NOT NULL DEFAULT 'member',  -- 'owner', 'admin', 'member'
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(organization_id, user_id)
);
```

### 3. Optional: Domain Tracking

For JIT provisioning based on email domain:

```sql
CREATE TABLE organization_domains (
    id              BIGINT PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organizations(id),
    domain          TEXT NOT NULL UNIQUE,  -- e.g., "acme.com"
    verified        BOOLEAN DEFAULT FALSE,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);
```

### 4. Optional: Invitations

```sql
CREATE TABLE organization_invitations (
    id              BIGINT PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organizations(id),
    email           TEXT NOT NULL,
    role            TEXT NOT NULL DEFAULT 'member',
    invited_by      BIGINT REFERENCES users(id),
    token           TEXT NOT NULL UNIQUE,
    expires_at      TIMESTAMPTZ NOT NULL,
    accepted_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);
```

---

## Model Changes

### Organization Model

```go
type Organization struct {
    ID                   int64     `json:"id"`
    Name                 string    `json:"name"`
    Slug                 string    `json:"slug"`
    WorkOSOrganizationID *string   `json:"workos_organization_id,omitempty"` // NEW
    CreatedAt            time.Time `json:"created_at"`
    UpdatedAt            time.Time `json:"updated_at"`
    IsDeleted            bool      `json:"-"`
}
```

### Membership Model

```go
type OrganizationMembership struct {
    ID             int64     `json:"id"`
    OrganizationID int64     `json:"organization_id"`
    UserID         int64     `json:"user_id"`
    Role           string    `json:"role"` // "owner", "admin", "member"
    CreatedAt      time.Time `json:"created_at"`
}
```

---

## Code Changes

### Update HandleCallback

When a user authenticates via SSO, WorkOS returns `OrganizationID`. Handle it:

```go
func (s *authService) HandleCallback(ctx context.Context, code string) (*model.User, *model.Session, error) {
    authResponse, _ := usermanagement.AuthenticateWithCode(ctx, ...)
    workosUser := authResponse.User
    
    // Upsert user (existing logic)
    user := s.upsertUser(ctx, workosUser)
    
    // NEW: Handle organization membership if SSO login
    if workosUser.OrganizationID != "" {
        s.handleOrgMembership(ctx, user, workosUser.OrganizationID)
    }
    
    // Create session (existing logic)
    ...
}

func (s *authService) handleOrgMembership(ctx context.Context, user *model.User, workosOrgID string) error {
    // Find org by WorkOS ID
    org, err := s.orgStore.GetByWorkOSID(ctx, workosOrgID)
    if err == store.ErrNotFound {
        // First user from this SSO org - auto-create the org
        workosOrg, _ := organizations.GetOrganization(ctx, workosOrgID)
        org = &model.Organization{
            ID:                   id.New(),
            Name:                 workosOrg.Name,
            Slug:                 slug.Generate(workosOrg.Name),
            WorkOSOrganizationID: &workosOrgID,
        }
        s.orgStore.Create(ctx, org)
    }
    
    // Add user to org if not already a member
    s.membershipStore.AddIfNotExists(ctx, org.ID, user.ID, "member")
    return nil
}
```

### New Store Methods

- `orgStore.GetByWorkOSID(ctx, workosOrgID)` - find org by WorkOS ID
- `membershipStore.AddIfNotExists(ctx, orgID, userID, role)` - idempotent add
- `membershipStore.GetByUser(ctx, userID)` - list user's orgs
- `membershipStore.GetByOrg(ctx, orgID)` - list org members

---

## First Customer Setup (Manual Process)

For the first enterprise customer, set up SSO manually:

1. Create WorkOS Organization in WorkOS dashboard
2. Add their domain (e.g., `customerdomain.com`)
3. Create SSO connection for JumpCloud
4. Exchange SAML metadata with customer IT admin
5. Test with a customer user account

Self-serve admin portal can come later.

---

## SSO Flow Overview

```
Non-SSO User (email/Google)          SSO User (JumpCloud)
----------------------------         --------------------
1. Auth via AuthKit                  1. Auth via JumpCloud
2. No OrganizationID returned        2. OrganizationID returned
3. User creates/joins org manually   3. Auto-linked to org
```

---

## Architecture Diagram

```
Your Database                        WorkOS
----------------                     ------
users                                Users
  workos_id --------------------------> id

organizations                        Organizations
  workos_organization_id --------------> id
                                          SSO Connection
organization_memberships                    JumpCloud
  (your membership tracking)
```

---

## Decisions Made

1. **Create WorkOS orgs on-demand** - only when customer needs SSO, not for every org
2. **Keep admin_user_id for now** - useful denormalization, migrate later if needed
3. **Membership table** - proper multi-user orgs, not just single admin
4. **Manual SSO setup first** - self-serve admin portal can come later

---

## Open Questions

- Multi-org support: Can one user belong to multiple organizations?
- Role granularity: Just owner/admin/member, or more complex?
- Invitation flow for non-SSO users?
- Migration path for existing orgs with admin_user_id?
