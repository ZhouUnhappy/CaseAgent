#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASE_URL="${CASEAGENT_BASE_URL:-http://localhost:8080/api/v1}"
POLL_ATTEMPTS="${CASEAGENT_POLL_ATTEMPTS:-30}"
POLL_INTERVAL_SECONDS="${CASEAGENT_POLL_INTERVAL_SECONDS:-2}"

REQUIREMENT_FIXTURE="${CASEAGENT_I1_REQUIREMENT_FIXTURE:-$ROOT_DIR/testdata/i1/requirement.md}"
PRODUCT_KNOWLEDGE_FIXTURE="${CASEAGENT_I1_PRODUCT_KNOWLEDGE_FIXTURE:-$ROOT_DIR/testdata/i1/product_knowledge.md}"
MODULE_KNOWLEDGE_FIXTURE="${CASEAGENT_I1_MODULE_KNOWLEDGE_FIXTURE:-$ROOT_DIR/testdata/i1/module_knowledge.md}"

DOCUMENT_QUERY="${CASEAGENT_I1_DOCUMENT_QUERY:-probe-gate-7781 rollback completed}"
KNOWLEDGE_QUERY="${CASEAGENT_I1_KNOWLEDGE_QUERY:-control-plane-probe 18080 probe-gate-7781}"

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
        -X POST \
        -d "$payload" \
        "$BASE_URL$path"
}

get_json() {
    local path="$1"

    curl --fail --silent --show-error --noproxy '*' "$BASE_URL$path"
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
        --arg name "I1 retrieval smoke $(date +%Y%m%d%H%M%S)" \
        --arg description "Created by scripts/i1_retrieval_smoke.sh" \
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
        --rawfile content "$fixture" \
        --argjson metadata '{"aliases":["I1 smoke fixture"],"keywords":["probe-gate-7781","control-plane-probe","18080"]}' \
        '{type: $type, name: $name, content: $content, metadata: $metadata}')"

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
        --argjson document_ids "[$DOCUMENT_ID]" \
        '{query: $query, top_k: 3, document_ids: $document_ids}')"

    local response
    response="$(post_json '/retrieval/documents' "$payload")"

    local matched_count
    matched_count="$(jq --argjson id "$DOCUMENT_ID" '[.items[] | select(.document_id == $id and (.matched_chunks | length > 0))] | length' <<<"$response")"
    if [ "$matched_count" -lt 1 ]; then
        echo "document retrieval did not return expected document $DOCUMENT_ID" >&2
        echo "$response" >&2
        exit 1
    fi

    log "document retrieval matched document $DOCUMENT_ID with query: $DOCUMENT_QUERY"
}

assert_knowledge_retrieval() {
    local payload
    payload="$(jq -n \
        --arg query "$KNOWLEDGE_QUERY" \
        '{query: $query, top_k: 5}')"

    local response
    response="$(post_json '/retrieval/knowledge' "$payload")"

    local matched_count
    matched_count="$(jq --argjson id "$MODULE_KNOWLEDGE_ID" '[.items[] | select(.id == $id)] | length' <<<"$response")"
    if [ "$matched_count" -lt 1 ]; then
        echo "knowledge retrieval did not return expected module knowledge $MODULE_KNOWLEDGE_ID" >&2
        echo "$response" >&2
        exit 1
    fi

    log "knowledge retrieval matched module knowledge $MODULE_KNOWLEDGE_ID with query: $KNOWLEDGE_QUERY"
}

assert_document_chunk_count() {
    if [ -z "${CASEAGENT_PSQL_DSN:-}" ]; then
        log "skipping document_chunks row-count check; set CASEAGENT_PSQL_DSN to enable it"
        return 0
    fi

    require_command psql

    local count
    count="$(psql "$CASEAGENT_PSQL_DSN" -Atc "SELECT count(*) FROM document_chunks WHERE document_id = $DOCUMENT_ID")"
    if [ "$count" -lt 1 ]; then
        echo "expected document_chunks rows for document $DOCUMENT_ID, got $count" >&2
        exit 1
    fi

    log "document_chunks row count for document $DOCUMENT_ID: $count"
}

main() {
    require_command curl
    require_command jq
    require_file "$REQUIREMENT_FIXTURE"
    require_file "$PRODUCT_KNOWLEDGE_FIXTURE"
    require_file "$MODULE_KNOWLEDGE_FIXTURE"

    log "base url: $BASE_URL"
    create_project
    upload_document
    upload_knowledge "product" "I1 CaseAgent Cloud" "$PRODUCT_KNOWLEDGE_FIXTURE" PRODUCT_KNOWLEDGE_ID
    upload_knowledge "module" "I1 Control Plane" "$MODULE_KNOWLEDGE_FIXTURE" MODULE_KNOWLEDGE_ID
    assert_document_chunk_count
    assert_document_retrieval
    assert_knowledge_retrieval

    log "I1 retrieval smoke passed"
    log "project_id=$PROJECT_ID document_id=$DOCUMENT_ID product_knowledge_id=$PRODUCT_KNOWLEDGE_ID module_knowledge_id=$MODULE_KNOWLEDGE_ID"
}

main "$@"
