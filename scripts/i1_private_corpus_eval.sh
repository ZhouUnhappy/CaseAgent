#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASE_URL="${CASEAGENT_BASE_URL:-http://localhost:8080/api/v1}"

# Private corpora must NOT silently fall into a default tenant. The operator
# has to name the tenant explicitly (see docs/multitenancy_plan.md §9.1).
TENANT_SLUG="${CASEAGENT_I1_PRIVATE_TENANT_SLUG:-}"
if [ -z "$TENANT_SLUG" ]; then
    echo "CASEAGENT_I1_PRIVATE_TENANT_SLUG is required (e.g. export CASEAGENT_I1_PRIVATE_TENANT_SLUG=internal-corp)" >&2
    exit 1
fi

# shellcheck source=lib/tenant.sh
. "$ROOT_DIR/scripts/lib/tenant.sh"

ARCH_DIR="${CASEAGENT_I1_PRIVATE_ARCH_DIR:-}"
INPUT_DIR="${CASEAGENT_I1_PRIVATE_INPUT_DIR:-}"
POLL_ATTEMPTS="${CASEAGENT_POLL_ATTEMPTS:-240}"
POLL_INTERVAL_SECONDS="${CASEAGENT_POLL_INTERVAL_SECONDS:-2}"
TOP_K="${CASEAGENT_I1_TOP_K:-5}"
RUN_TOKEN="${CASEAGENT_I1_RUN_TOKEN:-i1-private-$(date +%Y%m%d%H%M%S)-$RANDOM}"
REPORT_DIR="${CASEAGENT_I1_PRIVATE_REPORT_DIR:-$ROOT_DIR/testdata/private/runs}"
REPORT_PATH="$REPORT_DIR/$RUN_TOKEN.md"

PROJECT_ID=""
DOCUMENT_IDS=()
DOCUMENT_NAMES=()
KNOWLEDGE_IDS=()
KNOWLEDGE_NAMES=()

RAW_ARCH_BYTES=0
RAW_INPUT_BYTES=0
CLEANED_DOCUMENT_BYTES=0
CLEANED_KNOWLEDGE_BYTES=0
DOCUMENT_CHUNK_COUNT="skipped"
DOCUMENT_EMBEDDING_COUNT="skipped"
KNOWLEDGE_EMBEDDING_COUNT="skipped"

DOCUMENT_QUERIES=(
    "VDS IGMP Snooping 组播"
    "mcast-agent query 报文"
    "全局默认拒绝 组播"
)
KNOWLEDGE_QUERIES=(
    "VDS IGMP MLD Snooping"
    "OVS Bridge OpenFlow"
    "Everoute 分布式防火墙"
)
RETRIEVAL_SUMMARY=()

require_command() {
    if ! command -v "$1" >/dev/null 2>&1; then
        echo "missing required command: $1" >&2
        exit 1
    fi
}

require_dir() {
    if [ -z "$1" ] || [ ! -d "$1" ]; then
        echo "missing directory: $2" >&2
        exit 1
    fi
}

log() {
    printf '[i1-private] %s\n' "$1"
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

load_md_files() {
    local dir="$1"
    local array_name="$2"
    local file

    while IFS= read -r -d '' file; do
        eval "$array_name+=(\"\$file\")"
    done < <(find "$dir" -type f -name '*.md' -print0)
}

sum_file_bytes() {
    local total=0
    local size
    local file

    for file in "$@"; do
        size="$(wc -c <"$file" | tr -d ' ')"
        total=$((total + size))
    done

    printf '%s' "$total"
}

relative_path() {
    local base="$1"
    local file="$2"

    printf '%s' "${file#$base/}"
}

join_by_comma() {
    local IFS=,
    printf '%s' "$*"
}

json_number_array() {
    printf '%s\n' "$@" | jq -R 'select(length > 0) | tonumber' | jq -s '.'
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

    local deleted_knowledge
    local deleted_projects
    deleted_knowledge="$(psql "$CASEAGENT_PSQL_DSN" -At -c "DELETE FROM knowledge_base WHERE metadata @> '{\"aliases\":[\"I1 private corpus fixture\"]}' RETURNING id")"
    deleted_projects="$(psql "$CASEAGENT_PSQL_DSN" -At -c "DELETE FROM projects WHERE name LIKE 'I1 private corpus %' RETURNING id")"

    local knowledge_count=0
    local project_count=0
    if [ -n "$deleted_knowledge" ]; then
        knowledge_count=$(printf '%s\n' "$deleted_knowledge" | grep -c '^[0-9]' || true)
    fi
    if [ -n "$deleted_projects" ]; then
        project_count=$(printf '%s\n' "$deleted_projects" | grep -c '^[0-9]' || true)
    fi

    log "deleted $knowledge_count legacy private knowledge row(s) and $project_count legacy project row(s)"
}

create_project() {
    local payload
    payload="$(jq -n \
        --arg name "I1 private corpus $RUN_TOKEN" \
        --arg description "Created by scripts/i1_private_corpus_eval.sh" \
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
    local file="$1"
    local name="$2"
    local response

    response="$(curl --fail --silent --show-error --noproxy '*' \
        -H "X-Tenant-ID: $TENANT_SLUG" \
        -X POST \
        -F "name=$name" \
        -F 'type=markdown' \
        -F 'source=upload' \
        -F "file=@$file" \
        "$BASE_URL/projects/$PROJECT_ID/documents")"

    local id
    id="$(jq -r '.id' <<<"$response")"
    if [ -z "$id" ] || [ "$id" = "null" ]; then
        echo "failed to upload document $name: $response" >&2
        exit 1
    fi

    DOCUMENT_IDS+=("$id")
    DOCUMENT_NAMES+=("$name")
    log "uploaded document $id ($name)"
    poll_status "document" "/documents/$id" "$id"
    CLEANED_DOCUMENT_BYTES=$((CLEANED_DOCUMENT_BYTES + $(content_byte_count "/documents/$id")))
}

knowledge_type_for_path() {
    local relative="$1"

    case "$relative" in
        products/*)
            printf 'product'
            ;;
        *)
            printf 'module'
            ;;
    esac
}

upload_knowledge() {
    local file="$1"
    local relative="$2"
    local knowledge_type
    knowledge_type="$(knowledge_type_for_path "$relative")"

    local payload
    payload="$(jq -n \
        --arg type "$knowledge_type" \
        --arg name "$relative" \
        --arg run_token "$RUN_TOKEN" \
        --arg relative_path "$relative" \
        --rawfile content "$file" \
        '{
            type: $type,
            name: $name,
            content: $content,
            metadata: {
                aliases: ["I1 private corpus fixture"],
                run_token: $run_token,
                relative_path: $relative_path
            }
        }')"

    local response
    response="$(post_json '/knowledge' "$payload")"
    local id
    id="$(jq -r '.id' <<<"$response")"
    if [ -z "$id" ] || [ "$id" = "null" ]; then
        echo "failed to upload knowledge $relative: $response" >&2
        exit 1
    fi

    KNOWLEDGE_IDS+=("$id")
    KNOWLEDGE_NAMES+=("$relative")
    log "uploaded knowledge $id ($knowledge_type/$relative)"
    poll_status "knowledge" "/knowledge/$id" "$id"
    CLEANED_KNOWLEDGE_BYTES=$((CLEANED_KNOWLEDGE_BYTES + $(content_byte_count "/knowledge/$id")))
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

content_byte_count() {
    local path="$1"

    get_json "$path" | jq -r '.content // ""' | wc -c | tr -d ' '
}

assert_db_counts() {
    if [ -z "${CASEAGENT_PSQL_DSN:-}" ]; then
        log "skipping database chunk/embedding checks; set CASEAGENT_PSQL_DSN to enable"
        return 0
    fi

    require_command psql

    local doc_ids
    local knowledge_ids
    doc_ids="$(join_by_comma "${DOCUMENT_IDS[@]}")"
    knowledge_ids="$(join_by_comma "${KNOWLEDGE_IDS[@]}")"

    local doc_counts
    doc_counts="$(psql "$CASEAGENT_PSQL_DSN" -At -F $'\t' -c "SELECT count(*), count(*) FILTER (WHERE embedding IS NOT NULL), count(*) FILTER (WHERE embedding IS NULL) FROM document_chunks WHERE document_id IN ($doc_ids)")"
    DOCUMENT_CHUNK_COUNT="$(cut -f1 <<<"$doc_counts")"
    DOCUMENT_EMBEDDING_COUNT="$(cut -f2 <<<"$doc_counts")"
    local missing_doc_embeddings
    missing_doc_embeddings="$(cut -f3 <<<"$doc_counts")"

    if [ "$DOCUMENT_CHUNK_COUNT" -le 0 ]; then
        echo "expected private document chunks, got $DOCUMENT_CHUNK_COUNT" >&2
        exit 1
    fi
    if [ "$missing_doc_embeddings" -gt 0 ]; then
        echo "private document chunks have $missing_doc_embeddings NULL embedding(s)" >&2
        exit 1
    fi

    local knowledge_counts
    knowledge_counts="$(psql "$CASEAGENT_PSQL_DSN" -At -F $'\t' -c "SELECT count(*), count(*) FILTER (WHERE embedding IS NOT NULL), count(*) FILTER (WHERE embedding IS NULL) FROM knowledge_base WHERE id IN ($knowledge_ids)")"
    local knowledge_count
    knowledge_count="$(cut -f1 <<<"$knowledge_counts")"
    KNOWLEDGE_EMBEDDING_COUNT="$(cut -f2 <<<"$knowledge_counts")"
    local missing_knowledge_embeddings
    missing_knowledge_embeddings="$(cut -f3 <<<"$knowledge_counts")"

    if [ "$knowledge_count" -ne "${#KNOWLEDGE_IDS[@]}" ]; then
        echo "expected ${#KNOWLEDGE_IDS[@]} knowledge rows, got $knowledge_count" >&2
        exit 1
    fi
    if [ "$missing_knowledge_embeddings" -gt 0 ]; then
        echo "private knowledge rows have $missing_knowledge_embeddings NULL embedding(s)" >&2
        exit 1
    fi

    log "database checks passed: document_chunks=$DOCUMENT_CHUNK_COUNT, document_embeddings=$DOCUMENT_EMBEDDING_COUNT, knowledge_embeddings=$KNOWLEDGE_EMBEDDING_COUNT"
}

assert_document_retrieval() {
    local ids_json
    ids_json="$(json_number_array "${DOCUMENT_IDS[@]}")"

    local query
    for query in "${DOCUMENT_QUERIES[@]}"; do
        local payload
        payload="$(jq -n \
            --arg query "$query" \
            --argjson top_k "$TOP_K" \
            --argjson document_ids "$ids_json" \
            '{query: $query, top_k: $top_k, document_ids: $document_ids}')"

        local response
        response="$(post_json '/retrieval/documents' "$payload")"

        local first_id
        local first_name
        first_id="$(jq -r '.items[0].document_id // empty' <<<"$response")"
        first_name="$(jq -r '.items[0].name // empty' <<<"$response")"
        if [ -z "$first_id" ]; then
            echo "document retrieval returned no result for query: $query" >&2
            echo "$response" >&2
            exit 1
        fi

        RETRIEVAL_SUMMARY+=("document query '$query' -> rank-1 document_id=$first_id name=$first_name")
        log "document query hit rank-1 document $first_id ($first_name): $query"
    done
}

assert_knowledge_retrieval() {
    local ids_json
    ids_json="$(json_number_array "${KNOWLEDGE_IDS[@]}")"

    local query
    for query in "${KNOWLEDGE_QUERIES[@]}"; do
        local payload
        payload="$(jq -n \
            --arg query "$query" \
            --argjson top_k "$TOP_K" \
            '{query: $query, top_k: $top_k}')"

        local response
        response="$(post_json '/retrieval/knowledge' "$payload")"

        local rank
        rank="$(jq --argjson ids "$ids_json" '
            (.items // [])
            | to_entries
            | map(select(.value.id as $id | $ids | index($id)))
            | if length == 0 then empty else .[0].key + 1 end
        ' <<<"$response")"
        if [ -z "$rank" ]; then
            echo "knowledge retrieval did not hit current private knowledge in top $TOP_K for query: $query" >&2
            echo "(run_token=$RUN_TOKEN; consider CASEAGENT_I1_CLEANUP_LEGACY=1 if old private data is winning)" >&2
            echo "$response" >&2
            exit 1
        fi

        local hit
        hit="$(jq --argjson ids "$ids_json" '
            (.items // [])
            | to_entries
            | map(select(.value.id as $id | $ids | index($id)))
            | .[0].value
        ' <<<"$response")"
        local hit_id
        local hit_name
        hit_id="$(jq -r '.id' <<<"$hit")"
        hit_name="$(jq -r '.name' <<<"$hit")"

        RETRIEVAL_SUMMARY+=("knowledge query '$query' -> rank-$rank knowledge_id=$hit_id name=$hit_name")
        log "knowledge query hit current private knowledge $hit_id at rank $rank: $query"
    done
}

# Cross-tenant probe: query the same knowledge endpoint with a different
# X-Tenant-ID and assert none of this run's uploaded knowledge ids show up.
# Default probe tenant is i1-smoke; override with CASEAGENT_I1_PRIVATE_PROBE_TENANT.
assert_isolation_from_smoke() {
    local probe_tenant="${CASEAGENT_I1_PRIVATE_PROBE_TENANT:-i1-smoke}"
    if [ "$probe_tenant" = "$TENANT_SLUG" ]; then
        log "skipping cross-tenant probe: probe tenant == current ($TENANT_SLUG)"
        return 0
    fi
    ensure_tenant "$probe_tenant" "$probe_tenant"
    log "probing cross-tenant isolation from $probe_tenant (must not see this run's knowledge)"

    local ids_csv
    ids_csv="$(printf '%s,' "${KNOWLEDGE_IDS[@]}" | sed 's/,$//')"

    local query leaked
    for query in "${KNOWLEDGE_QUERIES[@]}"; do
        leaked="$(curl --fail --silent --show-error --noproxy '*' \
            -H 'Content-Type: application/json' \
            -H "X-Tenant-ID: $probe_tenant" \
            -X POST -d "$(jq -n --arg q "$query" --argjson topk "$TOP_K" '{query: $q, top_k: $topk}')" \
            "$BASE_URL/retrieval/knowledge" \
            | jq --argjson ids "[$ids_csv]" '[.items[] | select(.id as $i | $ids | index($i))] | length')"
        if [ "$leaked" -ne 0 ]; then
            echo "ISOLATION LEAK: tenant $probe_tenant saw $leaked private knowledge entries for query: $query" >&2
            exit 1
        fi
    done
    log "isolation OK: $probe_tenant cannot see any of this run's private knowledge"
}

write_report() {
    mkdir -p "$REPORT_DIR"

    {
        printf '# I1 Private Corpus Evaluation\n\n'
        printf -- '- run_token: `%s`\n' "$RUN_TOKEN"
        printf -- '- base_url: `%s`\n' "$BASE_URL"
        printf -- '- project_id: `%s`\n' "$PROJECT_ID"
        printf -- '- architecture_files: `%s`\n' "${#KNOWLEDGE_IDS[@]}"
        printf -- '- input_files: `%s`\n' "${#DOCUMENT_IDS[@]}"
        printf -- '- raw_architecture_bytes: `%s`\n' "$RAW_ARCH_BYTES"
        printf -- '- cleaned_knowledge_bytes: `%s`\n' "$CLEANED_KNOWLEDGE_BYTES"
        printf -- '- raw_input_bytes: `%s`\n' "$RAW_INPUT_BYTES"
        printf -- '- cleaned_document_bytes: `%s`\n' "$CLEANED_DOCUMENT_BYTES"
        printf -- '- document_chunks: `%s`\n' "$DOCUMENT_CHUNK_COUNT"
        printf -- '- document_embeddings: `%s`\n' "$DOCUMENT_EMBEDDING_COUNT"
        printf -- '- knowledge_embeddings: `%s`\n\n' "$KNOWLEDGE_EMBEDDING_COUNT"
        printf '## Retrieval\n\n'
        local item
        for item in "${RETRIEVAL_SUMMARY[@]}"; do
            printf -- '- %s\n' "$item"
        done
        printf '\n## Documents\n\n'
        local idx
        for idx in "${!DOCUMENT_IDS[@]}"; do
            printf -- '- document_id=`%s`, name=`%s`\n' "${DOCUMENT_IDS[$idx]}" "${DOCUMENT_NAMES[$idx]}"
        done
        printf '\n## Knowledge\n\n'
        for idx in "${!KNOWLEDGE_IDS[@]}"; do
            printf -- '- knowledge_id=`%s`, name=`%s`\n' "${KNOWLEDGE_IDS[$idx]}" "${KNOWLEDGE_NAMES[$idx]}"
        done
    } >"$REPORT_PATH"

    log "wrote private report: $REPORT_PATH"
}

main() {
    require_command curl
    require_command jq
    require_command find
    require_dir "$ARCH_DIR" "CASEAGENT_I1_PRIVATE_ARCH_DIR"
    require_dir "$INPUT_DIR" "CASEAGENT_I1_PRIVATE_INPUT_DIR"

    local arch_files=()
    local input_files=()
    load_md_files "$ARCH_DIR" arch_files
    load_md_files "$INPUT_DIR" input_files

    if [ "${#arch_files[@]}" -eq 0 ]; then
        echo "no markdown files found in CASEAGENT_I1_PRIVATE_ARCH_DIR=$ARCH_DIR" >&2
        exit 1
    fi
    if [ "${#input_files[@]}" -eq 0 ]; then
        echo "no markdown files found in CASEAGENT_I1_PRIVATE_INPUT_DIR=$INPUT_DIR" >&2
        exit 1
    fi

    RAW_ARCH_BYTES="$(sum_file_bytes "${arch_files[@]}")"
    RAW_INPUT_BYTES="$(sum_file_bytes "${input_files[@]}")"

    log "base url: $BASE_URL"
    log "run token: $RUN_TOKEN"
    log "tenant: $TENANT_SLUG"
    log "architecture files=${#arch_files[@]} raw_bytes=$RAW_ARCH_BYTES"
    log "input files=${#input_files[@]} raw_bytes=$RAW_INPUT_BYTES"

    ensure_tenant "$TENANT_SLUG" "I1 private corpus ($TENANT_SLUG)"
    cleanup_legacy
    create_project

    local file
    for file in "${arch_files[@]}"; do
        upload_knowledge "$file" "$(relative_path "$ARCH_DIR" "$file")"
    done
    for file in "${input_files[@]}"; do
        upload_document "$file" "$(relative_path "$INPUT_DIR" "$file")"
    done

    assert_db_counts
    assert_document_retrieval
    assert_knowledge_retrieval
    assert_isolation_from_smoke
    write_report

    log "I1 private corpus evaluation passed"
    log "run_token=$RUN_TOKEN project_id=$PROJECT_ID report=$REPORT_PATH"
}

main "$@"
