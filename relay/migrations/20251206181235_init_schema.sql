-- +goose Up
-- +goose StatementBegin
create table users (
    id bigint primary key,
    name text not null,
    email text not null unique,
    avatar_url text,

    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create index idx_users_email on users (email);


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

    provider text not null,
    provider_base_url text,

    external_org_id text,
    external_workspace_id text,

    access_token text not null, -- stored as encrypted string
    refresh_token text, -- stored as encrypted string
    expires_at timestamptz,

    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),

    -- There can only be one type of one integration of a provider for a workspace
    constraint unq_integrations_workspace_provider unique (workspace_id, provider)
);

create index idx_integrations_workspace_id on integrations (workspace_id);
create index idx_integrations_organization_id on integrations (organization_id);
create index idx_integrations_provider_external_org_id on integrations (provider, external_org_id);
create index idx_integrations_provider_external_workspace_id on integrations (provider, external_workspace_id);

create table repositories(
    id bigint primary key,
    workspace_id bigint not null references workspaces(id),
    integration_id bigint not null references integrations(id),

    name text not null,
    slug text not null,
    url text not null,
    description text,


    external_repo_id text not null, -- unique identifier for the repository in the provider

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


create table event_logs(
    id bigint primary key,
    workspace_id bigint not null references workspaces(id),

    source text not null,
    event_type text not null,

    payload text,
    external_id text,

    processed_at timestamptz,
    processing_error text,

    created_at timestamptz not null default now()
);

create index idx_event_logs_workspace_source on event_logs (workspace_id, source);
create index idx_event_logs_created_at on event_logs (created_at);

create table sessions (
    id              bigint primary key,
    user_id         bigint not null references users(id),
    created_at      timestamptz not null default now(),
    expires_at      timestamptz not null
);

create index idx_sessions_user_id on sessions (user_id);
create index idx_sessions_expires_at on sessions (expires_at);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

drop table if exists sessions;
drop table if exists event_logs;
drop table if exists integration_configs;
drop table if exists repositories;
drop table if exists integrations;
drop table if exists workspaces;
drop table if exists organizations;
drop table if exists users;

-- +goose StatementEnd
