#!/usr/bin/env bash
# Verify retrieval determinism for I2-T1: same fixture, same query, run 3 times,
# expect identical hit set + ordering for both document and knowledge retrieval.
#
# Usage:
#   bash scripts/i1_retrieval_determinism.sh
#
# Env:
#   CASEAGENT_BASE_URL                 (default http://localhost:8080/api/v1)
#   CASEAGENT_I1_DETERMINISM_DOC_IDS   comma list of document ids to constrain
#                                      the document search (default: all from
#                                      most recent public corpus run, queried
#                                      from db on the fly when CASEAGENT_PSQL_DSN
#                                      is set; otherwise required)
#   CASEAGENT_I1_DETERMINISM_REPORT    output path (default .dev/i1_retrieval_determinism.md)

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASE_URL="${CASEAGENT_BASE_URL:-http://localhost:8080/api/v1}"
REPORT="${CASEAGENT_I1_DETERMINISM_REPORT:-$ROOT_DIR/.dev/i1_retrieval_determinism.md}"
TOP_K="${CASEAGENT_I1_DETERMINISM_TOP_K:-5}"

# shellcheck source=lib/tenant.sh
. "$ROOT_DIR/scripts/lib/tenant.sh"

DOC_QUERIES=(
    "Dubbo 双注册原理 服务提供者 注册中心"
    "模块发布器 服务发布全过程 ServiceConfig"
    "Service Weaver 微服务编排 Google 论文"
)
KB_QUERIES=(
    "Dubbo 流量管理 路由规则"
    "Dubbo SPI 扩展点 加载机制"
    "Dubbo 回调参数 异步通知"
)

require_command() {
    if ! command -v "$1" >/dev/null 2>&1; then
        echo "missing required command: $1" >&2
        exit 1
    fi
}
require_command curl
require_command jq

resolve_document_ids() {
    if [ -n "${CASEAGENT_I1_DETERMINISM_DOC_IDS:-}" ]; then
        printf '%s' "$CASEAGENT_I1_DETERMINISM_DOC_IDS"
        return
    fi
    if [ -z "${CASEAGENT_PSQL_DSN:-}" ]; then
        echo "set CASEAGENT_I1_DETERMINISM_DOC_IDS='1,2,3' or CASEAGENT_PSQL_DSN to auto-resolve from latest public corpus run" >&2
        exit 1
    fi
    require_command psql
    psql "$CASEAGENT_PSQL_DSN" -At -c "
        SELECT string_agg(d.id::text, ',')
        FROM documents d
        JOIN projects p ON p.id = d.project_id
        WHERE p.name LIKE 'I1 public corpus %'
          AND p.id = (SELECT id FROM projects WHERE name LIKE 'I1 public corpus %' ORDER BY id DESC LIMIT 1)
    "
}

DOC_IDS_CSV="$(resolve_document_ids)"
if [ -z "$DOC_IDS_CSV" ]; then
    echo "no document ids resolved; run scripts/i1_public_corpus_eval.sh first or set CASEAGENT_I1_DETERMINISM_DOC_IDS" >&2
    exit 1
fi
DOC_IDS_JSON="$(printf '%s\n' "$DOC_IDS_CSV" | tr ',' '\n' | jq -R 'tonumber' | jq -s '.')"

FIRST_DOC_ID="$(printf '%s' "$DOC_IDS_CSV" | cut -d',' -f1)"
TENANT_SLUG="${CASEAGENT_TENANT_SLUG:-$(tenant_slug_for_document "$FIRST_DOC_ID")}"
if [ -z "$TENANT_SLUG" ]; then
    echo "could not resolve tenant for document $FIRST_DOC_ID (override via CASEAGENT_TENANT_SLUG)" >&2
    exit 1
fi
echo "[i1-determinism] tenant=$TENANT_SLUG document_ids=$DOC_IDS_CSV"

# fingerprint extracts the deterministic-relevant subset of an /retrieval/* response.
# Floating-point scores from the embedding API can micro-jitter between calls
# (same input -> slightly different vector); the DoD targets *hit set + ordering*,
# so the fingerprint deliberately excludes scores.
fingerprint_documents() {
    jq -S '[.items[] | {document_id, name, rank, hit_queries, matched_chunks: ([.matched_chunks[] | {query, rank}])}]'
}
fingerprint_knowledge() {
    jq -S '[.items[] | {id, name, rank, hit_queries}]'
}

# capture_score returns the rank-1 score for context (not part of fingerprint).
score_documents() { jq -r '.items[0].best_score // "n/a"'; }
score_knowledge() { jq -r '.items[0].score // "n/a"'; }
top_id_documents() { jq -r '.items[0].document_id // "n/a"'; }
top_id_knowledge() { jq -r '.items[0].id // "n/a"'; }

run_doc_query() {
    local query="$1"
    local payload
    payload="$(jq -n --arg q "$query" --argjson top_k "$TOP_K" --argjson document_ids "$DOC_IDS_JSON" \
        '{query: $q, top_k: $top_k, document_ids: $document_ids}')"
    curl --fail --silent --show-error --noproxy '*' \
        -H 'Content-Type: application/json' \
        -H "X-Tenant-ID: $TENANT_SLUG" \
        -X POST -d "$payload" \
        "$BASE_URL/retrieval/documents"
}

run_kb_query() {
    local query="$1"
    local payload
    payload="$(jq -n --arg q "$query" --argjson top_k "$TOP_K" \
        '{query: $q, top_k: $top_k}')"
    curl --fail --silent --show-error --noproxy '*' \
        -H 'Content-Type: application/json' \
        -H "X-Tenant-ID: $TENANT_SLUG" \
        -X POST -d "$payload" \
        "$BASE_URL/retrieval/knowledge"
}

mkdir -p "$(dirname "$REPORT")"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

failed=0
{
    printf '# I1 Retrieval Determinism (I2-T1)\n\n'
    printf -- '- base_url: `%s`\n' "$BASE_URL"
    printf -- '- top_k: `%s`\n' "$TOP_K"
    printf -- '- document_ids: `%s`\n\n' "$DOC_IDS_CSV"
    printf '## Document queries\n\n'
} > "$REPORT"

for query in "${DOC_QUERIES[@]}"; do
    for run in 1 2 3; do
        body=$(run_doc_query "$query")
        printf '%s' "$body" | fingerprint_documents > "$TMPDIR/doc.$run"
        if [ "$run" = "1" ]; then
            top_id=$(printf '%s' "$body" | top_id_documents)
            top_score=$(printf '%s' "$body" | score_documents)
        fi
    done
    if diff -q "$TMPDIR/doc.1" "$TMPDIR/doc.2" >/dev/null && diff -q "$TMPDIR/doc.1" "$TMPDIR/doc.3" >/dev/null; then
        printf -- '- ✓ `%s` → 3/3 identical hit set + ordering, rank-1 document_id=%s (best_score=%s on run 1)\n' \
            "$query" "$top_id" "$top_score" >> "$REPORT"
    else
        failed=1
        printf -- '- ✗ `%s` → DIFF in fingerprint across 3 runs\n' "$query" >> "$REPORT"
        diff "$TMPDIR/doc.1" "$TMPDIR/doc.2" | head -20 | sed 's/^/      /' >> "$REPORT" || true
    fi
done

printf '\n## Knowledge queries\n\n' >> "$REPORT"

for query in "${KB_QUERIES[@]}"; do
    for run in 1 2 3; do
        body=$(run_kb_query "$query")
        printf '%s' "$body" | fingerprint_knowledge > "$TMPDIR/kb.$run"
        if [ "$run" = "1" ]; then
            top_id=$(printf '%s' "$body" | top_id_knowledge)
            top_score=$(printf '%s' "$body" | score_knowledge)
        fi
    done
    if diff -q "$TMPDIR/kb.1" "$TMPDIR/kb.2" >/dev/null && diff -q "$TMPDIR/kb.1" "$TMPDIR/kb.3" >/dev/null; then
        printf -- '- ✓ `%s` → 3/3 identical hit set + ordering, rank-1 knowledge_id=%s (score=%s on run 1)\n' \
            "$query" "$top_id" "$top_score" >> "$REPORT"
    else
        failed=1
        printf -- '- ✗ `%s` → DIFF in fingerprint across 3 runs\n' "$query" >> "$REPORT"
        diff "$TMPDIR/kb.1" "$TMPDIR/kb.2" | head -20 | sed 's/^/      /' >> "$REPORT" || true
    fi
done

cat "$REPORT"
if [ "$failed" -ne 0 ]; then
    echo "determinism check FAILED" >&2
    exit 1
fi
echo "determinism check passed (report: $REPORT)"
