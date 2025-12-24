-- name: InsertLLMEval :one
INSERT INTO llm_evals (
    id, workspace_id, issue_id, stage,
    input_text, output_json,
    model, temperature, prompt_version,
    latency_ms, prompt_tokens, completion_tokens,
    created_at
) VALUES (
    $1, $2, $3, $4,
    $5, $6,
    $7, $8, $9,
    $10, $11, $12,
    NOW()
) RETURNING *;

-- name: GetLLMEvalByID :one
SELECT * FROM llm_evals WHERE id = $1;

-- name: ListLLMEvalsByIssue :many
SELECT * FROM llm_evals
WHERE issue_id = $1
ORDER BY created_at DESC;

-- name: ListLLMEvalsByStage :many
SELECT * FROM llm_evals
WHERE stage = $1
ORDER BY created_at DESC
LIMIT $2;

-- name: ListUnratedLLMEvals :many
SELECT * FROM llm_evals
WHERE rating IS NULL
  AND stage = $1
ORDER BY created_at DESC
LIMIT $2;

-- name: RateLLMEval :exec
UPDATE llm_evals
SET rating = $2,
    rating_notes = $3,
    rated_by_user_id = $4,
    rated_at = NOW()
WHERE id = $1;

-- name: SetLLMEvalExpected :exec
UPDATE llm_evals
SET expected_json = $2,
    eval_score = $3
WHERE id = $1;

-- name: GetEvalStats :one
SELECT
    stage,
    COUNT(*) as total,
    COUNT(rating) as rated,
    AVG(rating)::float as avg_rating,
    AVG(eval_score)::float as avg_eval_score,
    AVG(latency_ms)::int as avg_latency_ms
FROM llm_evals
WHERE stage = $1
  AND created_at > $2
GROUP BY stage;
