#!/usr/bin/env bash
# I2-T2 end-to-end probe: create a generation task on an existing public-corpus
# project, walk the analyze -> review -> generate pipeline, and assert that at
# least one test case landed in the database.
#
# Usage:
#   bash scripts/i2_generation_e2e.sh
#
# Env:
#   CASEAGENT_BASE_URL              default http://localhost:8080/api/v1
#   CASEAGENT_PSQL_DSN              required (used to resolve project + verify cases)
#   CASEAGENT_I2_E2E_PROJECT_ID     project to use; defaults to the most recent
#                                    "I1 public corpus %" project
#   CASEAGENT_I2_E2E_DOCUMENT_ID    single document id from the project; defaults
#                                    to the most recently uploaded completed doc
#   CASEAGENT_I2_E2E_REPORT         markdown output path; defaults to
#                                    .dev/i2_generation_e2e.md
#   CASEAGENT_I2_E2E_POLL_ATTEMPTS  default 240 (×2s polls per stage)

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASE_URL="${CASEAGENT_BASE_URL:-http://localhost:8080/api/v1}"
REPORT="${CASEAGENT_I2_E2E_REPORT:-$ROOT_DIR/.dev/i2_generation_e2e.md}"
TENANT_SLUG="${CASEAGENT_TENANT_SLUG:-apache-dubbo}"

# shellcheck source=lib/tenant.sh
. "$ROOT_DIR/scripts/lib/tenant.sh"
POLL_ATTEMPTS="${CASEAGENT_I2_E2E_POLL_ATTEMPTS:-600}"
POLL_INTERVAL_SECONDS="${CASEAGENT_I2_E2E_POLL_INTERVAL_SECONDS:-2}"

require_command() {
    if ! command -v "$1" >/dev/null 2>&1; then
        echo "missing required command: $1" >&2
        exit 1
    fi
}
require_command curl
require_command jq
require_command psql

if [ -z "${CASEAGENT_PSQL_DSN:-}" ]; then
    echo "CASEAGENT_PSQL_DSN is required (e.g. postgres://zhouxi@localhost:5432/eino_rag?sslmode=disable)" >&2
    exit 1
fi

resolve_project_id() {
    if [ -n "${CASEAGENT_I2_E2E_PROJECT_ID:-}" ]; then
        printf '%s' "$CASEAGENT_I2_E2E_PROJECT_ID"
        return
    fi
    psql_tenant "$TENANT_SLUG" "
        SELECT id FROM projects
        WHERE name LIKE 'I1 public corpus %'
        ORDER BY id DESC LIMIT 1
    "
}

resolve_document_id() {
    local pid="$1"
    if [ -n "${CASEAGENT_I2_E2E_DOCUMENT_ID:-}" ]; then
        printf '%s' "$CASEAGENT_I2_E2E_DOCUMENT_ID"
        return
    fi
    psql_tenant "$TENANT_SLUG" "
        SELECT id FROM documents
        WHERE project_id = $pid AND status = 'completed'
        ORDER BY id DESC LIMIT 1
    "
}

PROJECT_ID="$(resolve_project_id)"
if [ -z "$PROJECT_ID" ]; then
    echo "no public corpus project found; run scripts/i1_public_corpus_eval.sh first" >&2
    exit 1
fi
DOCUMENT_ID="$(resolve_document_id "$PROJECT_ID")"
if [ -z "$DOCUMENT_ID" ]; then
    echo "no completed documents in project $PROJECT_ID" >&2
    exit 1
fi

echo "[i2-e2e] project_id=$PROJECT_ID document_id=$DOCUMENT_ID tenant=$TENANT_SLUG"

post_json() {
    curl --fail --silent --show-error --noproxy '*' \
        -H 'Content-Type: application/json' \
        -H "X-Tenant-ID: $TENANT_SLUG" \
        -X POST -d "$2" "$BASE_URL$1"
}

put_json() {
    curl --fail --silent --show-error --noproxy '*' \
        -H 'Content-Type: application/json' \
        -H "X-Tenant-ID: $TENANT_SLUG" \
        -X PUT -d "$2" "$BASE_URL$1"
}

get_json() {
    curl --fail --silent --show-error --noproxy '*' \
        -H "X-Tenant-ID: $TENANT_SLUG" \
        "$BASE_URL$1"
}

poll_until_status() {
    local task_id="$1"
    local target="$2"  # regex of acceptable terminal statuses
    local status=""
    for ((i = 1; i <= POLL_ATTEMPTS; i++)); do
        status="$(get_json "/tasks/$task_id" | jq -r '.status')"
        if [[ "$status" =~ $target ]]; then
            printf '%s' "$status"
            return 0
        fi
        if [ "$status" = "failed" ] && [[ ! "$target" =~ failed ]]; then
            echo "task $task_id transitioned to failed unexpectedly" >&2
            return 1
        fi
        echo "[i2-e2e] task $task_id status=$status (waiting $i/$POLL_ATTEMPTS)" >&2
        sleep "$POLL_INTERVAL_SECONDS"
    done
    echo "task $task_id did not reach $target within timeout (last status=$status)" >&2
    return 1
}

# 1) Create task
create_payload="$(jq -n --argjson docs "[$DOCUMENT_ID]" '{document_ids: $docs}')"
task_response="$(post_json "/projects/$PROJECT_ID/tasks" "$create_payload")"
TASK_ID="$(jq -r '.id' <<<"$task_response")"
echo "[i2-e2e] created task $TASK_ID"

# 2) Wait for analyze -> awaiting_review
poll_until_status "$TASK_ID" '^(awaiting_review|ready_to_generate)$' >/dev/null

# 3) Review (accept whatever the analyzer inferred; if both empty, push a
# permissive scope so generation has at least the requirements context).
inferred="$(get_json "/tasks/$TASK_ID")"
products_json="$(jq -c '.affected_products // []' <<<"$inferred")"
modules_json="$(jq -c '.affected_modules // []' <<<"$inferred")"
review_payload="$(jq -n --argjson p "$products_json" --argjson m "$modules_json" \
    '{affected_products: $p, affected_modules: $m}')"
put_json "/tasks/$TASK_ID/review" "$review_payload" >/dev/null
echo "[i2-e2e] review submitted: products=$products_json modules=$modules_json"

# 4) Trigger generation
put_json "/tasks/$TASK_ID/generate" '{}' >/dev/null

# 5) Wait for terminal state
final_status="$(poll_until_status "$TASK_ID" '^(completed|failed)$')"
echo "[i2-e2e] task terminal status=$final_status"

# 6) Check cases. The /tasks/:id/cases endpoint returns one row per generated
# section. As of I2-T3, `cases` is a true JSONB array (no longer the
# double-encoded string from earlier builds) and each row carries a
# `source_context` JSONB blob recording the retrieval trace.
cases_response="$(get_json "/tasks/$TASK_ID/cases")"
section_count="$(jq 'length' <<<"$cases_response")"
case_count="$(jq '[.[] | (.cases // []) | length] | add // 0' <<<"$cases_response")"
sample_titles="$(jq -r '[.[] | (.cases // []) | .[].title] | .[0:5] | .[]?' <<<"$cases_response" 2>/dev/null || true)"

# I2-T3 invariants
duplicate_title_count="$(jq '
    [.[] | (.cases // []) | .[].title]
    | group_by(.)
    | map(select(length > 1))
    | length
' <<<"$cases_response")"

cases_missing_affected="$(jq '
    [.[] | (.cases // [])
        | .[]
        | select((.affected_products | type) != "array" or (.affected_modules | type) != "array")
    ] | length
' <<<"$cases_response")"

sections_with_source_ctx="$(jq '
    [.[] | select(.source_context != null and (.source_context | type) == "object")] | length
' <<<"$cases_response")"

source_context_summary="$(jq -r '
    .[0].source_context
    | if . == null then "<missing>" else
        "document_queries=\(.document_queries | length // 0)" +
        " knowledge_queries=\(.knowledge_queries | length // 0)" +
        " document_hits=\(.document_hits | length // 0)" +
        " knowledge_hits=\(.knowledge_hits | length // 0)" +
        " knowledge_shipped_ids=\(.knowledge_shipped_ids | length // 0)"
      end
' <<<"$cases_response")"

mkdir -p "$(dirname "$REPORT")"
{
    printf '# I2 Generation End-to-End (I2-T2 + I2-T3)\n\n'
    printf -- '- base_url: `%s`\n' "$BASE_URL"
    printf -- '- project_id: `%s`\n' "$PROJECT_ID"
    printf -- '- document_id: `%s`\n' "$DOCUMENT_ID"
    printf -- '- task_id: `%s`\n' "$TASK_ID"
    printf -- '- terminal_status: `%s`\n' "$final_status"
    printf -- '- section_count: `%s`\n' "$section_count"
    printf -- '- case_count: `%s`\n' "$case_count"
    printf -- '- duplicate_title_count: `%s` (I2-T3: expect 0)\n' "$duplicate_title_count"
    printf -- '- cases_missing_affected_fields: `%s` (I2-T3: expect 0)\n' "$cases_missing_affected"
    printf -- '- sections_with_source_context: `%s` of `%s`\n' "$sections_with_source_ctx" "$section_count"
    printf -- '- source_context[0] summary: `%s`\n' "$source_context_summary"
    printf '\n## First 5 case titles\n\n'
    if [ -n "$sample_titles" ]; then
        printf '%s\n' "$sample_titles" | sed 's/^/- /'
    else
        printf '_no titles available_\n'
    fi
} > "$REPORT"

cat "$REPORT"

if [ "$final_status" != "completed" ]; then
    echo "task did not complete (status=$final_status)" >&2
    exit 1
fi
if [ -z "$case_count" ] || [ "$case_count" = "null" ] || [ "$case_count" -lt 1 ]; then
    echo "expected at least 1 generated case, got $case_count" >&2
    exit 1
fi
if [ "$duplicate_title_count" != "0" ]; then
    echo "I2-T3 violation: $duplicate_title_count duplicate case title group(s) detected" >&2
    exit 1
fi
if [ "$cases_missing_affected" != "0" ]; then
    echo "I2-T3 violation: $cases_missing_affected case(s) missing affected_products/affected_modules" >&2
    exit 1
fi
if [ "$sections_with_source_ctx" != "$section_count" ]; then
    echo "I2-T3 violation: only $sections_with_source_ctx/$section_count sections have source_context" >&2
    exit 1
fi

echo "[i2-e2e] passed: $case_count case(s) across $section_count section(s) (report: $REPORT)"
