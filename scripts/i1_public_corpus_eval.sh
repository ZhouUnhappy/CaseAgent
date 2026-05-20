#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASE_URL="${CASEAGENT_BASE_URL:-http://localhost:8080/api/v1}"
TENANT_SLUG="${CASEAGENT_TENANT_SLUG:-apache-dubbo}"

# shellcheck source=lib/tenant.sh
. "$ROOT_DIR/scripts/lib/tenant.sh"

LONG_DIR="${CASEAGENT_I1_PUBLIC_LONG_DIR:-$ROOT_DIR/testdata/i1/public_corpus/long}"
SHORT_DIR="${CASEAGENT_I1_PUBLIC_SHORT_DIR:-$ROOT_DIR/testdata/i1/public_corpus/short}"
POLL_ATTEMPTS="${CASEAGENT_POLL_ATTEMPTS:-240}"
POLL_INTERVAL_SECONDS="${CASEAGENT_POLL_INTERVAL_SECONDS:-2}"
TOP_K="${CASEAGENT_I1_TOP_K:-5}"
RUN_TOKEN="${CASEAGENT_I1_RUN_TOKEN:-i1-public-$(date +%Y%m%d%H%M%S)-$RANDOM}"
REPORT_DIR="${CASEAGENT_I1_PUBLIC_REPORT_DIR:-$ROOT_DIR/testdata/i1/public_corpus/runs}"
REPORT_PATH="$REPORT_DIR/$RUN_TOKEN.md"

PROJECT_ID=""
DOCUMENT_IDS=()
DOCUMENT_NAMES=()
KNOWLEDGE_IDS=()
KNOWLEDGE_NAMES=()

RAW_LONG_BYTES=0
RAW_SHORT_BYTES=0
CLEANED_DOCUMENT_BYTES=0
CLEANED_KNOWLEDGE_BYTES=0
DOCUMENT_CHUNK_COUNT="skipped"
DOCUMENT_EMBEDDING_COUNT="skipped"
KNOWLEDGE_EMBEDDING_COUNT="skipped"

# Distinctive Chinese queries targeting Apache Dubbo content in the public corpus.
DOCUMENT_QUERIES=(
    "Dubbo 双注册原理 服务提供者 注册中心"
    "模块发布器 服务发布全过程 ServiceConfig"
    "Service Weaver 微服务编排 Google 论文"
)
KNOWLEDGE_QUERIES=(
    "Dubbo 流量管理 路由规则"
    "Dubbo SPI 扩展点 加载机制"
    "Dubbo 回调参数 异步通知"
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
    printf '[i1-public] %s\n' "$1"
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
    done < <(find "$dir" -maxdepth 1 -type f -name '*.md' ! -name 'SOURCES.md' -print0)
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
    deleted_knowledge="$(psql "$CASEAGENT_PSQL_DSN" -At -c "DELETE FROM knowledge_base WHERE metadata @> '{\"aliases\":[\"I1 public corpus fixture\"]}' RETURNING id")"
    deleted_projects="$(psql "$CASEAGENT_PSQL_DSN" -At -c "DELETE FROM projects WHERE name LIKE 'I1 public corpus %' RETURNING id")"

    local knowledge_count=0
    local project_count=0
    if [ -n "$deleted_knowledge" ]; then
        knowledge_count=$(printf '%s\n' "$deleted_knowledge" | grep -c '^[0-9]' || true)
    fi
    if [ -n "$deleted_projects" ]; then
        project_count=$(printf '%s\n' "$deleted_projects" | grep -c '^[0-9]' || true)
    fi

    log "deleted $knowledge_count legacy public knowledge row(s) and $project_count legacy project row(s)"
}

create_project() {
    local payload
    payload="$(jq -n \
        --arg name "I1 public corpus $RUN_TOKEN" \
        --arg description "Created by scripts/i1_public_corpus_eval.sh" \
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

upload_knowledge() {
    local file="$1"
    local relative="$2"

    local payload
    payload="$(jq -n \
        --arg type "module" \
        --arg name "$relative" \
        --arg run_token "$RUN_TOKEN" \
        --arg relative_path "$relative" \
        --rawfile content "$file" \
        '{
            type: $type,
            name: $name,
            content: $content,
            metadata: {
                aliases: ["I1 public corpus fixture"],
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
    log "uploaded knowledge $id ($relative)"
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
        echo "expected public document chunks, got $DOCUMENT_CHUNK_COUNT" >&2
        exit 1
    fi
    if [ "$missing_doc_embeddings" -gt 0 ]; then
        echo "public document chunks have $missing_doc_embeddings NULL embedding(s)" >&2
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
        echo "public knowledge rows have $missing_knowledge_embeddings NULL embedding(s)" >&2
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
            echo "knowledge retrieval did not hit current public knowledge in top $TOP_K for query: $query" >&2
            echo "(run_token=$RUN_TOKEN; consider CASEAGENT_I1_CLEANUP_LEGACY=1 if old public data is winning)" >&2
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
        log "knowledge query hit current public knowledge $hit_id at rank $rank: $query"
    done
}

write_report() {
    mkdir -p "$REPORT_DIR"

    {
        printf '# I1 Public Corpus Evaluation\n\n'
        printf -- '- run_token: `%s`\n' "$RUN_TOKEN"
        printf -- '- base_url: `%s`\n' "$BASE_URL"
        printf -- '- project_id: `%s`\n' "$PROJECT_ID"
        printf -- '- long_files: `%s`\n' "${#DOCUMENT_IDS[@]}"
        printf -- '- short_files: `%s`\n' "${#KNOWLEDGE_IDS[@]}"
        printf -- '- raw_long_bytes: `%s`\n' "$RAW_LONG_BYTES"
        printf -- '- cleaned_document_bytes: `%s`\n' "$CLEANED_DOCUMENT_BYTES"
        printf -- '- raw_short_bytes: `%s`\n' "$RAW_SHORT_BYTES"
        printf -- '- cleaned_knowledge_bytes: `%s`\n' "$CLEANED_KNOWLEDGE_BYTES"
        printf -- '- document_chunks: `%s`\n' "$DOCUMENT_CHUNK_COUNT"
        printf -- '- document_embeddings: `%s`\n' "$DOCUMENT_EMBEDDING_COUNT"
        printf -- '- knowledge_embeddings: `%s`\n\n' "$KNOWLEDGE_EMBEDDING_COUNT"
        printf '## Retrieval\n\n'
        local item
        for item in "${RETRIEVAL_SUMMARY[@]}"; do
            printf -- '- %s\n' "$item"
        done
        printf '\n## Documents (long)\n\n'
        local idx
        for idx in "${!DOCUMENT_IDS[@]}"; do
            printf -- '- document_id=`%s`, name=`%s`\n' "${DOCUMENT_IDS[$idx]}" "${DOCUMENT_NAMES[$idx]}"
        done
        printf '\n## Knowledge (short)\n\n'
        for idx in "${!KNOWLEDGE_IDS[@]}"; do
            printf -- '- knowledge_id=`%s`, name=`%s`\n' "${KNOWLEDGE_IDS[$idx]}" "${KNOWLEDGE_NAMES[$idx]}"
        done
    } >"$REPORT_PATH"

    log "wrote public report: $REPORT_PATH"
}

main() {
    require_command curl
    require_command jq
    require_command find
    require_dir "$LONG_DIR" "CASEAGENT_I1_PUBLIC_LONG_DIR"
    require_dir "$SHORT_DIR" "CASEAGENT_I1_PUBLIC_SHORT_DIR"

    local long_files=()
    local short_files=()
    load_md_files "$LONG_DIR" long_files
    load_md_files "$SHORT_DIR" short_files

    if [ "${#long_files[@]}" -eq 0 ]; then
        echo "no markdown files found in CASEAGENT_I1_PUBLIC_LONG_DIR=$LONG_DIR" >&2
        exit 1
    fi
    if [ "${#short_files[@]}" -eq 0 ]; then
        echo "no markdown files found in CASEAGENT_I1_PUBLIC_SHORT_DIR=$SHORT_DIR" >&2
        exit 1
    fi

    RAW_LONG_BYTES="$(sum_file_bytes "${long_files[@]}")"
    RAW_SHORT_BYTES="$(sum_file_bytes "${short_files[@]}")"

    log "base url: $BASE_URL"
    log "run token: $RUN_TOKEN"
    log "tenant: $TENANT_SLUG"
    log "long files=${#long_files[@]} raw_bytes=$RAW_LONG_BYTES"
    log "short files=${#short_files[@]} raw_bytes=$RAW_SHORT_BYTES"

    ensure_tenant "$TENANT_SLUG" "I1 public corpus"
    cleanup_legacy
    create_project

    local file
    for file in "${short_files[@]}"; do
        upload_knowledge "$file" "$(relative_path "$SHORT_DIR" "$file")"
    done
    for file in "${long_files[@]}"; do
        upload_document "$file" "$(relative_path "$LONG_DIR" "$file")"
    done

    assert_db_counts
    assert_document_retrieval
    assert_knowledge_retrieval
    write_report

    log "I1 public corpus evaluation passed"
    log "run_token=$RUN_TOKEN project_id=$PROJECT_ID report=$REPORT_PATH"
}

main "$@"
