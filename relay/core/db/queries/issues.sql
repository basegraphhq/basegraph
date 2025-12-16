-- name: UpsertIssue :one
INSERT INTO issues (
    id,
    integration_id,
    external_issue_id,
    title,
    description,
    labels,
    members,
    assignees,
    reporter,
    keywords,
    code_findings,
    learnings,
    discussions,
    spec,
    created_at,
    updated_at
)
VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, now(), now()
)
ON CONFLICT (integration_id, external_issue_id)
DO UPDATE
SET
    title = EXCLUDED.title,
    description = EXCLUDED.description,
    labels = EXCLUDED.labels,
    members = EXCLUDED.members,
    assignees = EXCLUDED.assignees,
    reporter = EXCLUDED.reporter,
    keywords = EXCLUDED.keywords,
    code_findings = EXCLUDED.code_findings,
    learnings = EXCLUDED.learnings,
    discussions = EXCLUDED.discussions,
    spec = EXCLUDED.spec,
    updated_at = now()
RETURNING *;

-- name: GetIssue :one
SELECT * FROM issues WHERE id = $1;

-- name: GetIssueByIntegrationAndExternalID :one
SELECT * FROM issues WHERE integration_id = $1 AND external_issue_id = $2;

