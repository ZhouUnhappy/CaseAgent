#!/usr/bin/env bash
# Bootstrap or reset a stable public demo run.
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
ACTION="${1:-bootstrap}"

REQUIREMENT_FIXTURE="${CASEAGENT_DEMO_REQUIREMENT_FIXTURE:-$ROOT_DIR/testdata/i1/requirement.md}"
PRODUCT_KNOWLEDGE_FIXTURE="${CASEAGENT_DEMO_PRODUCT_KNOWLEDGE_FIXTURE:-$ROOT_DIR/testdata/i1/product_knowledge.md}"
MODULE_KNOWLEDGE_FIXTURE="${CASEAGENT_DEMO_MODULE_KNOWLEDGE_FIXTURE:-$ROOT_DIR/testdata/i1/module_knowledge.md}"

PRODUCT_NAME="${CASEAGENT_DEMO_PRODUCT_NAME:-CaseAgent Cloud}"
MODULE_NAME="${CASEAGENT_DEMO_MODULE_NAME:-控制平面}"
PROJECT_PREFIX="${CASEAGENT_DEMO_PROJECT_PREFIX:-Demo CaseAgent}"
KNOWLEDGE_ALIAS="${CASEAGENT_DEMO_KNOWLEDGE_ALIAS:-CaseAgent demo fixture}"

PROJECT_ID=""
DOCUMENT_ID=""
PRODUCT_KNOWLEDGE_ID=""
MODULE_KNOWLEDGE_ID=""
TASK_ID=""

on_error() {
    local status="$1"
    local line="$2"

    {
        printf '\nDemo script failed\n'
        printf -- '- action: `%s`\n' "$ACTION"
        printf -- '- exit_status: `%s`\n' "$status"
        printf -- '- line: `%s`\n' "$line"
        printf -- '- api_url: `%s`\n' "$BASE_URL"
        printf -- '- tenant_slug: `%s`\n' "$TENANT_SLUG"
        printf -- '- run_token: `%s`\n' "$RUN_TOKEN"
        printf -- '- project_id: `%s`\n' "${PROJECT_ID:-}"
        printf -- '- document_id: `%s`\n' "${DOCUMENT_ID:-}"
        printf -- '- task_id: `%s`\n' "${TASK_ID:-}"
        if [ -n "${TASK_ID:-}" ]; then
            printf -- '- task_snapshot: '
            get_json "/tasks/$TASK_ID" 2>/dev/null || true
            printf '\n'
            printf -- '- trace_snapshot: '
            get_json "/tasks/$TASK_ID/trace" 2>/dev/null || true
            printf '\n'
        fi
    } >&2

    exit "$status"
}

trap 'on_error "$?" "$LINENO"' ERR

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

usage() {
    cat <<'EOF'
Usage:
  bash scripts/demo_bootstrap.sh [bootstrap|reset|fresh]

Commands:
  bootstrap  Create/reuse the demo tenant, import fixed fixtures, create a task,
             review impact scope, trigger generation, and print demo URLs.
  reset      Delete demo projects and demo knowledge from the demo tenant.
  fresh      Run reset, then bootstrap.

Environment:
  CASEAGENT_BASE_URL                 Default: http://localhost:40003/api/v1
  CASEAGENT_FRONTEND_URL             Default: http://localhost:40002
  CASEAGENT_TENANT_SLUG              Default: demo-caseagent
  CASEAGENT_DEMO_RUN_TOKEN           Default: demo-<timestamp>-<random>
  CASEAGENT_DEMO_*_FIXTURE           Override fixed markdown fixtures.
  CASEAGENT_DEMO_PRODUCT_NAME        Default: CaseAgent Cloud
  CASEAGENT_DEMO_MODULE_NAME         Default: 控制平面
EOF
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

delete_json() {
    local path="$1"

    curl --fail --silent --show-error --noproxy '*' \
        -H "X-Tenant-ID: $TENANT_SLUG" \
        -X DELETE \
        "$BASE_URL$path"
}

count_lines() {
    local text="$1"
    if [ -z "$text" ]; then
        printf '0'
        return
    fi
    printf '%s\n' "$text" | sed '/^$/d' | wc -l | tr -d ' '
}

demo_project_ids() {
    get_json '/projects' | jq -r --arg prefix "$PROJECT_PREFIX " '
        .[]
        | select(
            ((.name // "") | startswith($prefix)) or
            ((.description // "") | contains("scripts/demo_bootstrap.sh"))
          )
        | .id
    '
}

demo_knowledge_ids() {
    get_json '/knowledge' | jq -r --arg alias "$KNOWLEDGE_ALIAS" '
        .[]
        | select(((.metadata.aliases // []) | index($alias)) != null)
        | .id
    '
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
        --arg name "$PROJECT_PREFIX $RUN_TOKEN" \
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
        --arg alias "$KNOWLEDGE_ALIAS" \
        --rawfile content "$fixture" \
        '{
            type: $type,
            name: $name,
            content: $content,
            metadata: {
                aliases: [$alias],
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
    printf -- '- action: `%s`\n' "$ACTION"
    printf -- '- api_url: `%s`\n' "$BASE_URL"
    printf -- '- tenant_slug: `%s`\n' "$TENANT_SLUG"
    printf -- '- run_token: `%s`\n' "$RUN_TOKEN"
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

reset_demo() {
    log "resetting demo data in tenant=$TENANT_SLUG"
    ensure_tenant "$TENANT_SLUG" "CaseAgent Demo"

    local project_ids knowledge_ids deleted_projects deleted_knowledge
    project_ids="$(demo_project_ids)"
    knowledge_ids="$(demo_knowledge_ids)"
    deleted_projects=0
    deleted_knowledge=0

    while IFS= read -r project_id; do
        [ -z "$project_id" ] && continue
        log "deleting demo project $project_id"
        delete_json "/projects/$project_id" >/dev/null
        deleted_projects=$((deleted_projects + 1))
    done <<<"$project_ids"

    while IFS= read -r knowledge_id; do
        [ -z "$knowledge_id" ] && continue
        log "deleting demo knowledge $knowledge_id"
        delete_json "/knowledge/$knowledge_id" >/dev/null
        deleted_knowledge=$((deleted_knowledge + 1))
    done <<<"$knowledge_ids"

    printf '\nDemo reset complete\n'
    printf -- '- action: `reset`\n'
    printf -- '- api_url: `%s`\n' "$BASE_URL"
    printf -- '- tenant_slug: `%s`\n' "$TENANT_SLUG"
    printf -- '- matched_projects: `%s`\n' "$(count_lines "$project_ids")"
    printf -- '- matched_knowledge: `%s`\n' "$(count_lines "$knowledge_ids")"
    printf -- '- deleted_projects: `%s`\n' "$deleted_projects"
    printf -- '- deleted_knowledge: `%s`\n' "$deleted_knowledge"
    printf -- '- cleanup_scope: `projects/documents/tasks/test_cases via API cascade; demo knowledge via API`\n'
}

bootstrap_demo() {
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

main() {
    require_command curl
    require_command jq

    case "$ACTION" in
        bootstrap | "")
            bootstrap_demo
            ;;
        reset)
            reset_demo
            ;;
        fresh)
            reset_demo
            bootstrap_demo
            ;;
        -h | --help | help)
            usage
            ;;
        *)
            usage >&2
            exit 2
            ;;
    esac
}

main "$@"
