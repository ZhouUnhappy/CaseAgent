#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASE_URL="${CASEAGENT_BASE_URL:-http://localhost:40003/api/v1}"
TENANT_SLUG="${CASEAGENT_TENANT_SLUG:-i1-smoke}"

# shellcheck source=lib/tenant.sh
. "$ROOT_DIR/scripts/lib/tenant.sh"

POLL_ATTEMPTS="${CASEAGENT_POLL_ATTEMPTS:-30}"
POLL_INTERVAL_SECONDS="${CASEAGENT_POLL_INTERVAL_SECONDS:-2}"

REQUIREMENT_FIXTURE="${CASEAGENT_I1_REQUIREMENT_FIXTURE:-$ROOT_DIR/testdata/i1/requirement.md}"
PRODUCT_KNOWLEDGE_FIXTURE="${CASEAGENT_I1_PRODUCT_KNOWLEDGE_FIXTURE:-$ROOT_DIR/testdata/i1/product_knowledge.md}"
MODULE_KNOWLEDGE_FIXTURE="${CASEAGENT_I1_MODULE_KNOWLEDGE_FIXTURE:-$ROOT_DIR/testdata/i1/module_knowledge.md}"

DOCUMENT_QUERY="${CASEAGENT_I1_DOCUMENT_QUERY:-probe-gate-7781 rollback completed}"
KNOWLEDGE_QUERY="${CASEAGENT_I1_KNOWLEDGE_QUERY:-control-plane-probe 18080 probe-gate-7781}"
TOP_K="${CASEAGENT_I1_TOP_K:-5}"
RUN_TOKEN="${CASEAGENT_I1_RUN_TOKEN:-i1-$(date +%Y%m%d%H%M%S)-$RANDOM}"

PROJECT_ID=""
DOCUMENT_ID=""
PRODUCT_KNOWLEDGE_ID=""
MODULE_KNOWLEDGE_ID=""

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
    printf '[i1-smoke] %s\n' "$1"
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

get_json() {
    local path="$1"

    curl --fail --silent --show-error --noproxy '*' \
        -H "X-Tenant-ID: $TENANT_SLUG" \
        "$BASE_URL$path"
}

poll_status() {
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
                return 1
                ;;
        esac

        log "$kind $id status=$status, waiting ($attempt/$POLL_ATTEMPTS)"
        sleep "$POLL_INTERVAL_SECONDS"
    done

    echo "$kind $id did not complete within timeout; last status=$status" >&2
    return 1
}

create_project() {
    local payload
    payload="$(jq -n \
        --arg name "I1 retrieval smoke $RUN_TOKEN" \
        --arg description "Created by scripts/i1_retrieval_smoke.sh (run_token=$RUN_TOKEN)" \
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
        -F 'name=I1 Requirement Fixture' \
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
    poll_status "document" "/documents/$DOCUMENT_ID" "$DOCUMENT_ID"
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
                aliases: ["I1 smoke fixture"],
                keywords: ["probe-gate-7781", "control-plane-probe", "18080"],
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
    poll_status "knowledge" "/knowledge/$id" "$id"
}

assert_document_retrieval() {
    local payload
    payload="$(jq -n \
        --arg query "$DOCUMENT_QUERY" \
        --argjson top_k "$TOP_K" \
        --argjson document_ids "[$DOCUMENT_ID]" \
        '{query: $query, top_k: $top_k, document_ids: $document_ids}')"

    local response
    response="$(post_json '/retrieval/documents' "$payload")"

    local first_id
    first_id="$(jq -r '.items[0].document_id // empty' <<<"$response")"
    if [ "$first_id" != "$DOCUMENT_ID" ]; then
        echo "document retrieval expected document $DOCUMENT_ID at rank 1, got '$first_id'" >&2
        echo "$response" >&2
        exit 1
    fi

    local first_chunk_count
    first_chunk_count="$(jq '(.items[0].matched_chunks // []) | length' <<<"$response")"
    if [ "$first_chunk_count" -lt 1 ]; then
        echo "document retrieval rank-1 entry has no matched_chunks" >&2
        echo "$response" >&2
        exit 1
    fi

    log "document retrieval: document $DOCUMENT_ID ranked 1st (top_k=$TOP_K) for query: $DOCUMENT_QUERY"
}

assert_knowledge_retrieval() {
    local payload
    payload="$(jq -n \
        --arg query "$KNOWLEDGE_QUERY" \
        --argjson top_k "$TOP_K" \
        '{query: $query, top_k: $top_k}')"

    local response
    response="$(post_json '/retrieval/knowledge' "$payload")"

    local first_id
    first_id="$(jq -r '.items[0].id // empty' <<<"$response")"
    if [ "$first_id" != "$MODULE_KNOWLEDGE_ID" ]; then
        echo "knowledge retrieval expected module knowledge $MODULE_KNOWLEDGE_ID at rank 1, got '$first_id'" >&2
        echo "(run_token=$RUN_TOKEN; if old data is winning the rank, drop legacy knowledge_base rows or filter by metadata.run_token)" >&2
        echo "$response" >&2
        exit 1
    fi

    local first_run_token
    first_run_token="$(jq -r '.items[0].metadata.run_token // empty' <<<"$response")"
    if [ "$first_run_token" != "$RUN_TOKEN" ]; then
        echo "knowledge retrieval rank-1 metadata.run_token mismatch: expected $RUN_TOKEN, got '$first_run_token'" >&2
        echo "$response" >&2
        exit 1
    fi

    log "knowledge retrieval: module knowledge $MODULE_KNOWLEDGE_ID ranked 1st (top_k=$TOP_K, run_token=$RUN_TOKEN)"
}

assert_document_chunk_count() {
    if [ -z "${CASEAGENT_PSQL_DSN:-}" ]; then
        log "skipping document_chunks/embedding row checks; set CASEAGENT_PSQL_DSN to enable"
        return 0
    fi

    require_command psql

    local count
    count="$(psql_tenant "$TENANT_SLUG" "SELECT count(*) FROM document_chunks WHERE document_id = $DOCUMENT_ID")"
    if [ "$count" -lt 1 ]; then
        echo "expected document_chunks rows for document $DOCUMENT_ID, got $count" >&2
        exit 1
    fi

    local missing
    missing="$(psql_tenant "$TENANT_SLUG" "SELECT count(*) FROM document_chunks WHERE document_id = $DOCUMENT_ID AND embedding IS NULL")"
    if [ "$missing" -gt 0 ]; then
        echo "document $DOCUMENT_ID has $missing chunks with NULL embedding" >&2
        exit 1
    fi

    log "document_chunks for document $DOCUMENT_ID: count=$count, all embeddings non-null"
}

assert_knowledge_embedding() {
    if [ -z "${CASEAGENT_PSQL_DSN:-}" ]; then
        return 0
    fi

    require_command psql

    local missing
    missing="$(psql_tenant "$TENANT_SLUG" "SELECT count(*) FROM knowledge_base WHERE id IN ($PRODUCT_KNOWLEDGE_ID, $MODULE_KNOWLEDGE_ID) AND embedding IS NULL")"
    if [ "$missing" -gt 0 ]; then
        echo "$missing knowledge entries (of {$PRODUCT_KNOWLEDGE_ID, $MODULE_KNOWLEDGE_ID}) have NULL embedding" >&2
        exit 1
    fi

    log "knowledge_base embeddings non-null for {$PRODUCT_KNOWLEDGE_ID, $MODULE_KNOWLEDGE_ID}"
}

main() {
    require_command curl
    require_command jq
    require_file "$REQUIREMENT_FIXTURE"
    require_file "$PRODUCT_KNOWLEDGE_FIXTURE"
    require_file "$MODULE_KNOWLEDGE_FIXTURE"

    log "base url: $BASE_URL"
    log "run token: $RUN_TOKEN"
    log "tenant: $TENANT_SLUG"

    ensure_tenant "$TENANT_SLUG" "I1 smoke"

    if [ "${CASEAGENT_I1_CLEANUP_LEGACY:-0}" = "1" ]; then
        if [ -z "${CASEAGENT_PSQL_DSN:-}" ]; then
            echo "CASEAGENT_I1_CLEANUP_LEGACY=1 requires CASEAGENT_PSQL_DSN" >&2
            exit 1
        fi
        log "running legacy cleanup before smoke"
        bash "$ROOT_DIR/scripts/i1_retrieval_cleanup.sh"
    fi

    create_project
    upload_document
    upload_knowledge "product" "I1 CaseAgent Cloud" "$PRODUCT_KNOWLEDGE_FIXTURE" PRODUCT_KNOWLEDGE_ID
    upload_knowledge "module" "I1 Control Plane" "$MODULE_KNOWLEDGE_FIXTURE" MODULE_KNOWLEDGE_ID
    assert_document_chunk_count
    assert_knowledge_embedding
    assert_document_retrieval
    assert_knowledge_retrieval

    log "I1 retrieval smoke passed"
    log "run_token=$RUN_TOKEN project_id=$PROJECT_ID document_id=$DOCUMENT_ID product_knowledge_id=$PRODUCT_KNOWLEDGE_ID module_knowledge_id=$MODULE_KNOWLEDGE_ID"
}

main "$@"
