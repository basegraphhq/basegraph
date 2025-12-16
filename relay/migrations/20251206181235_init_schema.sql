-- +goose Up
-- +goose StatementBegin
create table users (
    id bigint primary key,
    name text not null,
    email text not null unique,
    avatar_url text,
    workos_id text unique,

    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create index idx_users_email on users (email);
create index idx_users_workos_id on users (workos_id) where workos_id is not null;


create table organizations (
    id bigint primary key,
    admin_user_id bigint not null references users(id),

    name text not null,
    slug text not null unique,

    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),

    is_deleted boolean not null default false
);

create index idx_organizations_slug on organizations (slug);


create table workspaces(
    id bigint primary key,
    admin_user_id bigint not null references users(id), -- TODO: @nithinsj - re-think this
    organization_id bigint not null references organizations(id),
    user_id bigint not null references users(id),

    name text not null,
    slug text not null,
    description text,
    
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),

    is_deleted boolean not null default false,

    constraint unq_workspaces_org_slug unique (organization_id, slug)
);

create index idx_workspaces_org_id on workspaces (organization_id);
create index idx_workspaces_admin_user_id on workspaces (admin_user_id);
-- TODO: @nithinsj - Add index on is_deleted later


create table integrations(
    id bigint primary key,
    workspace_id bigint not null references workspaces(id),
    organization_id bigint not null references organizations(id),
    setup_by_user_id bigint not null references users(id),

    provider text not null,
    capabilities text[] not null default '{}',
    provider_base_url text,

    external_org_id text,
    external_workspace_id text,

    is_enabled boolean not null default true,

    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),

    constraint unq_integrations_workspace_provider unique (workspace_id, provider)
);

create index idx_integrations_workspace_id on integrations (workspace_id);
create index idx_integrations_organization_id on integrations (organization_id);
create index idx_integrations_setup_by_user_id on integrations (setup_by_user_id);
create index idx_integrations_provider_external_org_id on integrations (provider, external_org_id);
create index idx_integrations_provider_external_workspace_id on integrations (provider, external_workspace_id);


create table integration_credentials(
    id bigint primary key,
    integration_id bigint not null references integrations(id),
    user_id bigint references users(id),

    credential_type text not null,
    
    access_token text not null,
    refresh_token text,
    token_expires_at timestamptz,
    scopes text[],

    is_primary boolean not null default false,

    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    revoked_at timestamptz
);

create index idx_integration_credentials_integration_id on integration_credentials (integration_id);
create index idx_integration_credentials_user_id on integration_credentials (user_id) where user_id is not null;
create index idx_integration_credentials_is_primary on integration_credentials (integration_id, is_primary) where is_primary = true;


create table repositories(
    id bigint primary key,
    workspace_id bigint not null references workspaces(id),
    integration_id bigint not null references integrations(id),

    name text not null,
    slug text not null,
    url text not null,
    description text,


    external_repo_id text not null,

    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),

    constraint unq_repositories_external_repo_id unique (integration_id, external_repo_id)
);

create index idx_repositories_integration_id on repositories (integration_id);
create index idx_repositories_workspace_id on repositories (workspace_id);
create index idx_repositories_external_repo_id on repositories (external_repo_id);


create table integration_configs(
    id bigint primary key,
    integration_id bigint not null references integrations(id),

    key text not null,
    value jsonb not null,
    config_type text not null,

    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create index idx_integration_configs_integration_id on integration_configs (integration_id);
create index idx_integration_configs_config_type on integration_configs (config_type);


create table issues (
    id bigint primary key,
    integration_id bigint not null references integrations(id),
    external_issue_id text not null,

    title text,
    description text,
    labels text[],
    members text[],
    assignees text[],
    reporter text,

    keywords text[],
    code_findings jsonb,
    learnings jsonb,
    discussions jsonb,
    spec text,

    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),

    constraint unq_issues_integration_external_issue_id unique (integration_id, external_issue_id)
);

create index idx_issues_integration_id on issues (integration_id);


create table event_logs(
    id bigint primary key,
    workspace_id bigint not null references workspaces(id),
    issue_id bigint not null references issues(id),

    source text not null,
    event_type text not null,

    payload jsonb,
    external_id text,
    dedupe_key text not null,

    processed_at timestamptz,
    processing_error text,

    created_at timestamptz not null default now()
);

create index idx_event_logs_workspace_source on event_logs (workspace_id, source);
create index idx_event_logs_created_at on event_logs (created_at);
create unique index unq_event_logs_workspace_dedupe_key on event_logs (workspace_id, dedupe_key);


create table pipeline_runs (
    id bigint primary key,
    event_log_id bigint not null references event_logs(id),
    attempt int not null default 1,
    status text not null,
    error text,
    started_at timestamptz not null default now(),
    finished_at timestamptz
);

create index idx_pipeline_runs_event_log_id on pipeline_runs (event_log_id);
create index idx_pipeline_runs_status on pipeline_runs (status);

create table sessions (
    id              bigint primary key,
    user_id         bigint not null references users(id),
    created_at      timestamptz not null default now(),
    expires_at      timestamptz not null
);

create index idx_sessions_user_id on sessions (user_id);
create index idx_sessions_expires_at on sessions (expires_at);


create table learnings (
    id bigint primary key,
    workspace_id bigint not null references workspaces(id),
    rule_updated_by_issue_id bigint references issues(id), -- NULL for workspace-level rules
    type text not null check (type in ('project_standards', 'codebase_standards', 'domain_knowledge')),
    content text not null,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    
    constraint unq_workspace_type_content unique (workspace_id, type, content)
);

create index idx_learnings_workspace_id on learnings (workspace_id);
create index idx_learnings_type on learnings (type);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

drop table if exists learnings;
drop table if exists sessions;
drop table if exists pipeline_runs;
drop table if exists event_logs;
drop table if exists issues;
drop table if exists integration_configs;
drop table if exists repositories;
drop table if exists integration_credentials;
drop table if exists integrations;
drop table if exists workspaces;
drop table if exists organizations;
drop table if exists users;

-- +goose StatementEnd
