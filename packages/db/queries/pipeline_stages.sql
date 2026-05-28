-- pipeline_stages.sql — read queries for workspace pipeline stages.
--
-- Queries run with search_path = workspace_<uuid>, so table names are unqualified.
-- Stage writes are owned by the provisioner; v0 only exposes reads.
-- UpdateDealStage lives in deals.sql.

-- name: ListPipelineStages :many
SELECT id, name, order_index, created_at
FROM pipeline_stages
ORDER BY order_index ASC, name ASC;

-- name: GetPipelineStage :one
SELECT id, name, order_index, created_at
FROM pipeline_stages
WHERE id = $1;
