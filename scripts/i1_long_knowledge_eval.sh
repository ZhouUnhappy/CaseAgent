#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASE_URL="${CASEAGENT_BASE_URL:-http://localhost:40003/api/v1}"
TENANT_SLUG="${CASEAGENT_TENANT_SLUG:-i1-long-knowledge}"

# shellcheck source=lib/tenant.sh
. "$ROOT_DIR/scripts/lib/tenant.sh"

POLL_ATTEMPTS="${CASEAGENT_POLL_ATTEMPTS:-30}"
POLL_INTERVAL_SECONDS="${CASEAGENT_POLL_INTERVAL_SECONDS:-2}"
LONG_KNOWLEDGE_FIXTURE="${CASEAGENT_I1_LONG_KNOWLEDGE_FIXTURE:-$ROOT_DIR/testdata/i1/long_knowledge.md}"
TOP_K="${CASEAGENT_I1_TOP_K:-5}"
RUN_TOKEN="${CASEAGENT_I1_RUN_TOKEN:-i1-long-$(date +%Y%m%d%H%M%S)-$RANDOM}"

LONG_KNOWLEDGE_ID=""
QUERY_LABELS=(
    "A"
    "B"
    "C"
)
QUERIES=(
    "ingest-relay-3209 19090 数据集成"
    "notify-center-9921 21100 通知"
    "billing-pulse-4477 23330 计费"
)

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
    printf '[i1-long-knowledge] %s\n' "$1"
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

cleanup_legacy() {
    if [ "${CASEAGENT_I1_CLEANUP_LEGACY:-0}" != "1" ]; then
        return 0
    fi
    if [ -z "${CASEAGENT_PSQL_DSN:-}" ]; then
        echo "CASEAGENT_I1_CLEANUP_LEGACY=1 requires CASEAGENT_PSQL_DSN" >&2
        exit 1
    fi

    require_command psql

    local pred
    pred="metadata @> '{\"aliases\":[\"I1 long knowledge fixture\"]}'"

    local deleted
    deleted="$(psql "$CASEAGENT_PSQL_DSN" -At -c "DELETE FROM knowledge_base WHERE $pred RETURNING id")"
    local deleted_count=0
    if [ -n "$deleted" ]; then
        deleted_count=$(printf '%s\n' "$deleted" | grep -c '^[0-9]' || true)
    fi
    log "deleted $deleted_count legacy long knowledge row(s)"
}

upload_long_knowledge() {
    local payload
    payload="$(jq -n \
        --arg type "module" \
        --arg name "I1 Long Knowledge Fixture" \
        --arg run_token "$RUN_TOKEN" \
        --rawfile content "$LONG_KNOWLEDGE_FIXTURE" \
        '{
            type: $type,
            name: $name,
            content: $content,
            metadata: {
                aliases: ["I1 long knowledge fixture"],
                keywords: [
                    "ingest-relay-3209",
                    "notify-center-9921",
                    "billing-pulse-4477"
                ],
                run_token: $run_token
            }
        }')"

    local response
    response="$(post_json '/knowledge' "$payload")"
    LONG_KNOWLEDGE_ID="$(jq -r '.id' <<<"$response")"
    if [ -z "$LONG_KNOWLEDGE_ID" ] || [ "$LONG_KNOWLEDGE_ID" = "null" ]; then
        echo "failed to upload long knowledge: $response" >&2
        exit 1
    fi

    log "uploaded long knowledge $LONG_KNOWLEDGE_ID"
}

poll_knowledge_status() {
    local status=""

    for ((attempt = 1; attempt <= POLL_ATTEMPTS; attempt++)); do
        status="$(get_json "/knowledge/$LONG_KNOWLEDGE_ID" | jq -r '.status')"
        case "$status" in
            completed)
                log "knowledge $LONG_KNOWLEDGE_ID completed"
                return 0
                ;;
            failed)
                echo "knowledge $LONG_KNOWLEDGE_ID failed" >&2
                return 1
                ;;
        esac

        log "knowledge $LONG_KNOWLEDGE_ID status=$status, waiting ($attempt/$POLL_ATTEMPTS)"
        sleep "$POLL_INTERVAL_SECONDS"
    done

    echo "knowledge $LONG_KNOWLEDGE_ID did not complete within timeout; last status=$status" >&2
    return 1
}

assert_embedding_written() {
    if [ -z "${CASEAGENT_PSQL_DSN:-}" ]; then
        log "skipping knowledge embedding row check; set CASEAGENT_PSQL_DSN to enable"
        return 0
    fi

    require_command psql

    local missing
    missing="$(psql_tenant "$TENANT_SLUG" "SELECT count(*) FROM knowledge_base WHERE id = $LONG_KNOWLEDGE_ID AND embedding IS NULL")"
    if [ "$missing" -gt 0 ]; then
        echo "knowledge $LONG_KNOWLEDGE_ID has NULL embedding" >&2
        exit 1
    fi

    log "knowledge_base embedding non-null for $LONG_KNOWLEDGE_ID"
}

assert_query_hits_long_knowledge() {
    local label="$1"
    local query="$2"
    local payload
    payload="$(jq -n \
        --arg query "$query" \
        --argjson top_k "$TOP_K" \
        '{query: $query, top_k: $top_k}')"

    local response
    response="$(post_json '/retrieval/knowledge' "$payload")"

    local rank
    rank="$(jq --arg id "$LONG_KNOWLEDGE_ID" '
        (.items // [])
        | to_entries
        | map(select((.value.id | tostring) == $id))
        | if length == 0 then empty else .[0].key + 1 end
    ' <<<"$response")"

    if [ -z "$rank" ]; then
        echo "query $label expected long knowledge $LONG_KNOWLEDGE_ID in top $TOP_K, but it was absent" >&2
        echo "$response" >&2
        exit 1
    fi

    log "query $label hit long knowledge $LONG_KNOWLEDGE_ID at rank $rank (top_k=$TOP_K): $query"
}

main() {
    require_command curl
    require_command jq
    require_file "$LONG_KNOWLEDGE_FIXTURE"

    log "base url: $BASE_URL"
    log "run token: $RUN_TOKEN"
    log "tenant: $TENANT_SLUG"

    ensure_tenant "$TENANT_SLUG" "I1 long knowledge"

    cleanup_legacy
    upload_long_knowledge
    poll_knowledge_status
    assert_embedding_written

    for idx in "${!QUERIES[@]}"; do
        assert_query_hits_long_knowledge "${QUERY_LABELS[$idx]}" "${QUERIES[$idx]}"
    done

    log "I1 long knowledge whole-entry embedding evaluation passed"
    log "run_token=$RUN_TOKEN long_knowledge_id=$LONG_KNOWLEDGE_ID"
}

main "$@"
