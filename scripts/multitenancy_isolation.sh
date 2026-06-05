#!/usr/bin/env bash
# Multi-tenancy isolation smoke: verify two equal-rank tenants cannot see
# each other's knowledge under RLS.
#
# Creates two throw-away tenants (iso-a-* / iso-b-*), uploads the same fixture
# into each, then searches from each side and asserts the other side's record
# is NOT in the result set. Complements i1_private_corpus_eval.sh's reverse
# assertion (private vs cross-tenant): this covers private vs private.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASE_URL="${CASEAGENT_BASE_URL:-http://localhost:40003/api/v1}"
FIXTURE="${CASEAGENT_ISOLATION_FIXTURE:-$ROOT_DIR/testdata/i1/product_knowledge.md}"
POLL_ATTEMPTS="${CASEAGENT_POLL_ATTEMPTS:-60}"
POLL_INTERVAL_SECONDS="${CASEAGENT_POLL_INTERVAL_SECONDS:-2}"
QUERY="${CASEAGENT_ISOLATION_QUERY:-control-plane-probe}"
RUN_TOKEN="iso-$(date +%Y%m%d%H%M%S)-$RANDOM"
TENANT_A="iso-a-$RUN_TOKEN"
TENANT_B="iso-b-$RUN_TOKEN"

# shellcheck source=lib/tenant.sh
. "$ROOT_DIR/scripts/lib/tenant.sh"

require_command() {
    if ! command -v "$1" >/dev/null 2>&1; then
        echo "missing required command: $1" >&2
        exit 1
    fi
}

log() {
    printf '[isolation] %s\n' "$1"
}

upload_knowledge() {
    local tenant="$1"
    local name="$2"

    local payload
    payload="$(jq -n \
        --arg type "module" \
        --arg name "$name" \
        --arg token "$RUN_TOKEN" \
        --rawfile content "$FIXTURE" \
        '{type: $type, name: $name, content: $content, metadata: {aliases: ["multitenancy isolation"], run_token: $token}}')"

    local response
    response="$(curl --fail --silent --show-error --noproxy '*' \
        -H 'Content-Type: application/json' \
        -H "X-Tenant-ID: $tenant" \
        -X POST -d "$payload" "$BASE_URL/knowledge")"
    local id
    id="$(jq -r '.id' <<<"$response")"
    if [ -z "$id" ] || [ "$id" = "null" ]; then
        echo "upload to $tenant failed: $response" >&2
        exit 1
    fi

    for ((attempt = 1; attempt <= POLL_ATTEMPTS; attempt++)); do
        local status
        status="$(curl --fail --silent --show-error --noproxy '*' \
            -H "X-Tenant-ID: $tenant" "$BASE_URL/knowledge/$id" | jq -r '.status')"
        case "$status" in
            completed)
                printf '%s' "$id"
                return 0
                ;;
            failed)
                echo "knowledge $id ($tenant) failed" >&2
                exit 1
                ;;
        esac
        sleep "$POLL_INTERVAL_SECONDS"
    done

    echo "knowledge $id ($tenant) did not complete" >&2
    exit 1
}

search_knowledge_ids() {
    local tenant="$1"
    curl --fail --silent --show-error --noproxy '*' \
        -H 'Content-Type: application/json' \
        -H "X-Tenant-ID: $tenant" \
        -X POST -d "$(jq -n --arg q "$QUERY" '{query: $q, top_k: 10}')" \
        "$BASE_URL/retrieval/knowledge" \
        | jq -r '[.items[].id] | @csv'
}

main() {
    require_command curl
    require_command jq
    [ -f "$FIXTURE" ] || { echo "fixture missing: $FIXTURE" >&2; exit 1; }

    log "base url: $BASE_URL"
    log "tenants: $TENANT_A, $TENANT_B"
    log "fixture: $FIXTURE"

    ensure_tenant "$TENANT_A" "Isolation A"
    ensure_tenant "$TENANT_B" "Isolation B"

    local id_a id_b
    id_a="$(upload_knowledge "$TENANT_A" "iso-fixture-a")"
    id_b="$(upload_knowledge "$TENANT_B" "iso-fixture-b")"
    log "uploaded $id_a (tenant A) and $id_b (tenant B)"

    local hits_a hits_b
    hits_a="$(search_knowledge_ids "$TENANT_A")"
    hits_b="$(search_knowledge_ids "$TENANT_B")"
    log "tenant A retrieval hits: [$hits_a]"
    log "tenant B retrieval hits: [$hits_b]"

    if grep -q "\b$id_b\b" <<<"$hits_a"; then
        echo "ISOLATION LEAK: tenant A retrieval saw tenant B's knowledge id=$id_b" >&2
        exit 1
    fi
    if grep -q "\b$id_a\b" <<<"$hits_b"; then
        echo "ISOLATION LEAK: tenant B retrieval saw tenant A's knowledge id=$id_a" >&2
        exit 1
    fi

    log "multitenancy isolation verified: A and B cannot see each other's knowledge"
}

main "$@"
