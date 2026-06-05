#!/usr/bin/env bash
# Bootstrap a stable public demo run.
#
# Default tenant: demo-caseagent. This script only uses repository fixtures and
# keeps demo data out of private corpus tenants.
#
# Expected backend config for stable demos:
#   model.chat.provider: fake
#   model.chat.model: valid_json
#   model.embedding.provider: fake

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASE_URL="${CASEAGENT_BASE_URL:-http://localhost:40003/api/v1}"
FRONTEND_URL="${CASEAGENT_FRONTEND_URL:-http://localhost:40002}"
TENANT_SLUG="${CASEAGENT_TENANT_SLUG:-demo-caseagent}"

# shellcheck source=lib/tenant.sh
. "$ROOT_DIR/scripts/lib/tenant.sh"

POLL_ATTEMPTS="${CASEAGENT_DEMO_POLL_ATTEMPTS:-240}"
POLL_INTERVAL_SECONDS="${CASEAGENT_DEMO_POLL_INTERVAL_SECONDS:-2}"
RUN_TOKEN="${CASEAGENT_DEMO_RUN_TOKEN:-demo-$(date +%Y%m%d%H%M%S)-$RANDOM}"

REQUIREMENT_FIXTURE="${CASEAGENT_DEMO_REQUIREMENT_FIXTURE:-$ROOT_DIR/testdata/i1/requirement.md}"
PRODUCT_KNOWLEDGE_FIXTURE="${CASEAGENT_DEMO_PRODUCT_KNOWLEDGE_FIXTURE:-$ROOT_DIR/testdata/i1/product_knowledge.md}"
MODULE_KNOWLEDGE_FIXTURE="${CASEAGENT_DEMO_MODULE_KNOWLEDGE_FIXTURE:-$ROOT_DIR/testdata/i1/module_knowledge.md}"

PRODUCT_NAME="${CASEAGENT_DEMO_PRODUCT_NAME:-CaseAgent Cloud}"
MODULE_NAME="${CASEAGENT_DEMO_MODULE_NAME:-控制平面}"

PROJECT_ID=""
DOCUMENT_ID=""
PRODUCT_KNOWLEDGE_ID=""
MODULE_KNOWLEDGE_ID=""
TASK_ID=""

require_command() {
    if ! command -v "$1" >/dev/null 2>&1; then
        echo "missing required command: $1" >&2
        exit 1
    fi
}

require_file() {
    if [ ! -f "$1" ]; then
        echo "missing fixture: $1" >&2
        exit 1
    fi
}

log() {
    printf '[demo-bootstrap] %s\n' "$1"
}

post_json() {
    local path="$1"
    local payload="$2"

    curl --fail --silent --show-error --noproxy '*' \
        -H 'Content-Type: application/json' \
        -H "X-Tenant-ID: $TENANT_SLUG" \
        -X POST \
        -d "$payload" \
        "$BASE_URL$path"
}

put_json() {
    local path="$1"
    local payload="$2"

    curl --fail --silent --show-error --noproxy '*' \
        -H 'Content-Type: application/json' \
        -H "X-Tenant-ID: $TENANT_SLUG" \
        -X PUT \
        -d "$payload" \
        "$BASE_URL$path"
}

get_json() {
    local path="$1"

    curl --fail --silent --show-error --noproxy '*' \
        -H "X-Tenant-ID: $TENANT_SLUG" \
        "$BASE_URL$path"
}

poll_resource_completed() {
    local kind="$1"
    local path="$2"
    local id="$3"
    local status=""

    for ((attempt = 1; attempt <= POLL_ATTEMPTS; attempt++)); do
        status="$(get_json "$path" | jq -r '.status')"
        case "$status" in
            completed)
                log "$kind $id completed"
                return 0
                ;;
            failed)
                echo "$kind $id failed" >&2
                get_json "$path" >&2 || true
                return 1
                ;;
        esac

        log "$kind $id status=$status, waiting ($attempt/$POLL_ATTEMPTS)"
        sleep "$POLL_INTERVAL_SECONDS"
    done

    echo "$kind $id did not complete within timeout; last status=$status" >&2
    return 1
}

poll_task_status() {
    local task_id="$1"
    local target_regex="$2"
    local status=""

    for ((attempt = 1; attempt <= POLL_ATTEMPTS; attempt++)); do
        status="$(get_json "/tasks/$task_id" | jq -r '.status')"
        if [[ "$status" =~ $target_regex ]]; then
            printf '%s' "$status"
            return 0
        fi
        if [ "$status" = "failed" ] && [[ ! "failed" =~ $target_regex ]]; then
            echo "task $task_id failed before reaching $target_regex" >&2
            get_json "/tasks/$task_id/trace" >&2 || true
            return 1
        fi

        log "task $task_id status=$status, waiting ($attempt/$POLL_ATTEMPTS)"
        sleep "$POLL_INTERVAL_SECONDS"
    done

    echo "task $task_id did not reach $target_regex within timeout; last status=$status" >&2
    return 1
}

create_project() {
    local payload
    payload="$(jq -n \
        --arg name "Demo CaseAgent $RUN_TOKEN" \
        --arg description "Created by scripts/demo_bootstrap.sh (run_token=$RUN_TOKEN)" \
        '{name: $name, description: $description}')"

    local response
    response="$(post_json '/projects' "$payload")"
    PROJECT_ID="$(jq -r '.id' <<<"$response")"
    if [ -z "$PROJECT_ID" ] || [ "$PROJECT_ID" = "null" ]; then
        echo "failed to create project: $response" >&2
        exit 1
    fi

    log "created project $PROJECT_ID"
}

upload_document() {
    local response
    response="$(curl --fail --silent --show-error --noproxy '*' \
        -H "X-Tenant-ID: $TENANT_SLUG" \
        -X POST \
        -F "name=Demo Requirement $RUN_TOKEN" \
        -F 'type=markdown' \
        -F 'source=upload' \
        -F "file=@$REQUIREMENT_FIXTURE" \
        "$BASE_URL/projects/$PROJECT_ID/documents")"

    DOCUMENT_ID="$(jq -r '.id' <<<"$response")"
    if [ -z "$DOCUMENT_ID" ] || [ "$DOCUMENT_ID" = "null" ]; then
        echo "failed to upload document: $response" >&2
        exit 1
    fi

    log "uploaded document $DOCUMENT_ID"
    poll_resource_completed "document" "/documents/$DOCUMENT_ID" "$DOCUMENT_ID"
}

upload_knowledge() {
    local knowledge_type="$1"
    local knowledge_name="$2"
    local fixture="$3"
    local id_var="$4"

    local payload
    payload="$(jq -n \
        --arg type "$knowledge_type" \
        --arg name "$knowledge_name" \
        --arg run_token "$RUN_TOKEN" \
        --rawfile content "$fixture" \
        '{
            type: $type,
            name: $name,
            content: $content,
            metadata: {
                aliases: ["CaseAgent demo fixture"],
                run_token: $run_token
            }
        }')"

    local response
    response="$(post_json '/knowledge' "$payload")"
    local id
    id="$(jq -r '.id' <<<"$response")"
    if [ -z "$id" ] || [ "$id" = "null" ]; then
        echo "failed to upload knowledge: $response" >&2
        exit 1
    fi

    printf -v "$id_var" '%s' "$id"
    log "uploaded knowledge $id ($knowledge_type/$knowledge_name)"
    poll_resource_completed "knowledge" "/knowledge/$id" "$id"
}

create_task() {
    local payload
    payload="$(jq -n --argjson docs "[$DOCUMENT_ID]" '{document_ids: $docs}')"

    local response
    response="$(post_json "/projects/$PROJECT_ID/tasks" "$payload")"
    TASK_ID="$(jq -r '.id' <<<"$response")"
    if [ -z "$TASK_ID" ] || [ "$TASK_ID" = "null" ]; then
        echo "failed to create task: $response" >&2
        exit 1
    fi

    log "created task $TASK_ID"
}

review_task_scope() {
    poll_task_status "$TASK_ID" '^(awaiting_review|ready_to_generate)$' >/dev/null

    local task_response products_json modules_json review_payload
    task_response="$(get_json "/tasks/$TASK_ID")"
    products_json="$(jq -c --arg fallback "$PRODUCT_NAME" '(.affected_products // []) | if length == 0 then [$fallback] else . end' <<<"$task_response")"
    modules_json="$(jq -c --arg fallback "$MODULE_NAME" '(.affected_modules // []) | if length == 0 then [$fallback] else . end' <<<"$task_response")"
    review_payload="$(jq -n --argjson products "$products_json" --argjson modules "$modules_json" \
        '{affected_products: $products, affected_modules: $modules}')"

    put_json "/tasks/$TASK_ID/review" "$review_payload" >/dev/null
    log "review submitted: products=$products_json modules=$modules_json"
}

generate_cases() {
    put_json "/tasks/$TASK_ID/generate" '{}' >/dev/null

    local final_status
    final_status="$(poll_task_status "$TASK_ID" '^(completed|failed)$')"
    log "task terminal status=$final_status"
    if [ "$final_status" != "completed" ]; then
        get_json "/tasks/$TASK_ID/trace" >&2 || true
        exit 1
    fi
}

assert_demo_ready() {
    local cases_response trace_response case_count section_count model_call_count agent_run_count
    cases_response="$(get_json "/tasks/$TASK_ID/cases")"
    trace_response="$(get_json "/tasks/$TASK_ID/trace")"
    section_count="$(jq 'length' <<<"$cases_response")"
    case_count="$(jq '[.[] | (.cases // []) | length] | add // 0' <<<"$cases_response")"
    model_call_count="$(jq '(.model_calls // []) | length' <<<"$trace_response")"
    agent_run_count="$(jq '(.agent_runs // []) | length' <<<"$trace_response")"

    if [ -z "$case_count" ] || [ "$case_count" = "null" ] || [ "$case_count" -lt 1 ]; then
        echo "expected at least one generated case, got $case_count" >&2
        echo "$cases_response" >&2
        exit 1
    fi
    if [ "$model_call_count" -lt 1 ]; then
        echo "expected trace to include model_calls, got $model_call_count" >&2
        echo "$trace_response" >&2
        exit 1
    fi

    printf '\nDemo ready\n'
    printf -- '- tenant_slug: `%s`\n' "$TENANT_SLUG"
    printf -- '- project_id: `%s`\n' "$PROJECT_ID"
    printf -- '- document_id: `%s`\n' "$DOCUMENT_ID"
    printf -- '- product_knowledge_id: `%s`\n' "$PRODUCT_KNOWLEDGE_ID"
    printf -- '- module_knowledge_id: `%s`\n' "$MODULE_KNOWLEDGE_ID"
    printf -- '- task_id: `%s`\n' "$TASK_ID"
    printf -- '- frontend_url: `%s/tasks/%s`\n' "$FRONTEND_URL" "$TASK_ID"
    printf -- '- sections: `%s`\n' "$section_count"
    printf -- '- cases: `%s`\n' "$case_count"
    printf -- '- agent_runs: `%s`\n' "$agent_run_count"
    printf -- '- model_calls: `%s`\n' "$model_call_count"
    printf -- '- local_storage_hint: `localStorage.setItem("caseagent.tenant_slug", "%s")`\n' "$TENANT_SLUG"
}

main() {
    require_command curl
    require_command jq
    require_file "$REQUIREMENT_FIXTURE"
    require_file "$PRODUCT_KNOWLEDGE_FIXTURE"
    require_file "$MODULE_KNOWLEDGE_FIXTURE"

    log "base_url=$BASE_URL tenant=$TENANT_SLUG run_token=$RUN_TOKEN"
    ensure_tenant "$TENANT_SLUG" "CaseAgent Demo"
    create_project
    upload_document
    upload_knowledge "product" "$PRODUCT_NAME" "$PRODUCT_KNOWLEDGE_FIXTURE" PRODUCT_KNOWLEDGE_ID
    upload_knowledge "module" "$MODULE_NAME" "$MODULE_KNOWLEDGE_FIXTURE" MODULE_KNOWLEDGE_ID
    create_task
    review_task_scope
    generate_cases
    assert_demo_ready
}

main "$@"
