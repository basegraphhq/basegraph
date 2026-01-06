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
    external_project_id text,
    external_issue_id text not null,
    provider text not null,

    title text,
    description text,
    labels text[],
    members text[],
    assignees text[],
    reporter text,
    external_issue_url text,

    keywords jsonb not null default '[]'::jsonb,
    code_findings jsonb,
    learnings jsonb,
    discussions jsonb,
    spec text,

    processing_status text not null default 'idle',
    processing_started_at timestamptz,
    last_processed_at timestamptz,

    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),

    constraint unq_issues_integration_external_issue_id unique (integration_id, external_issue_id)
);

comment on column issues.processing_status is 'idle | queued | processing';

alter table issues add constraint chk_issues_processing_status
    check (processing_status in ('idle', 'queued', 'processing'));

create index idx_issues_integration_id on issues (integration_id);
create index idx_issues_processing_status on issues (processing_status) where processing_status != 'idle';



create table event_logs(
    id bigint primary key,
    workspace_id bigint not null references workspaces(id),
    issue_id bigint not null references issues(id),
    triggered_by_username text not null default 'system',

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
create index idx_event_logs_issue_unprocessed on event_logs (issue_id, created_at) where processed_at is null;

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
    short_id bigserial not null,
    workspace_id bigint not null references workspaces(id),
    rule_updated_by_issue_id bigint references issues(id), -- NULL for workspace-level rules
    type text not null check (type in ('domain_learnings', 'code_learnings')),
    content text not null,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    
    constraint unq_workspace_type_content unique (workspace_id, type, content)
);

create index idx_learnings_workspace_id on learnings (workspace_id);
create index idx_learnings_type on learnings (type);
create unique index unq_learnings_short_id on learnings (short_id);


create table gaps (
    id bigint primary key,
    short_id bigserial not null,
    issue_id bigint not null references issues(id),
    status text not null default 'open',
    closed_reason text,
    closed_note text,
    question text not null,
    evidence text,
    severity text not null,
    respondent text not null,
    learning_id bigint references learnings(id),
    created_at timestamptz not null default now(),
    resolved_at timestamptz
);

comment on column gaps.status is 'open | resolved | skipped';
comment on column gaps.closed_reason is 'answered | inferred | not_relevant';
comment on column gaps.severity is 'blocking | high | medium | low';
comment on column gaps.respondent is 'reporter | assignee';

alter table gaps add constraint chk_gaps_status
    check (status in ('open', 'resolved', 'skipped'));

alter table gaps add constraint chk_gaps_severity
    check (severity in ('blocking', 'high', 'medium', 'low'));

alter table gaps add constraint chk_gaps_respondent
    check (respondent in ('reporter', 'assignee'));

create index idx_gaps_issue_id on gaps (issue_id);
create index idx_gaps_status on gaps (status) where status = 'open';
create index idx_gaps_issue_status on gaps (issue_id, status);
create unique index unq_gaps_short_id on gaps (short_id);


-- LLM Pipeline Evaluation Logs
-- Stores inputs/outputs for quality analysis and prompt iteration
create table llm_evals (
    id bigint primary key,
    workspace_id bigint references workspaces(id),
    issue_id bigint references issues(id),

    -- Pipeline stage: keywords, planner, gap_detector, spec_generator
    stage text not null,

    -- Input/Output capture
    input_text text not null,
    output_json jsonb not null,

    -- Model config at time of generation
    model text not null,
    temperature float,
    prompt_version text, -- e.g., "keywords_v1", "keywords_v2"

    -- Performance
    latency_ms int,
    prompt_tokens int,
    completion_tokens int,

    -- Human evaluation (filled later)
    rating int check (rating >= 1 and rating <= 5),
    rating_notes text,
    rated_by_user_id bigint references users(id),
    rated_at timestamptz,

    -- Ground truth comparison (for automated evals)
    expected_json jsonb,
    eval_score float, -- computed metric (e.g., F1 score)

    created_at timestamptz not null default now()
);

create index idx_llm_evals_stage on llm_evals (stage);
create index idx_llm_evals_issue_id on llm_evals (issue_id);
create index idx_llm_evals_created_at on llm_evals (created_at);
create index idx_llm_evals_unrated on llm_evals (stage, created_at) where rating is null;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

drop table if exists llm_evals;
drop table if exists gaps;
drop table if exists learnings;
drop table if exists sessions;
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
