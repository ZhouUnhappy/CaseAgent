#!/usr/bin/env bash

# scripts/i1_retrieval_cleanup.sh
# Remove legacy I1 smoke knowledge_base rows so retrieval rank assertions stay clean.
# Selection: rows whose metadata.aliases contains "I1 smoke fixture".
#
# Usage:
#   CASEAGENT_PSQL_DSN='postgres://user:pass@host:5432/db?sslmode=disable' \
#     bash scripts/i1_retrieval_cleanup.sh [--dry-run]

set -euo pipefail

print_usage() {
    cat <<'EOF'
Usage:
  CASEAGENT_PSQL_DSN='postgres://...' bash scripts/i1_retrieval_cleanup.sh [--dry-run]

Removes prior I1 smoke knowledge_base rows (metadata.aliases ⊇ ["I1 smoke fixture"])
to keep retrieval rank assertions clean.

  --dry-run   Print candidate rows but do not delete.
EOF
}

DRY_RUN=0
while [ $# -gt 0 ]; do
    case "$1" in
        --dry-run)
            DRY_RUN=1
            shift
            ;;
        -h|--help)
            print_usage
            exit 0
            ;;
        *)
            echo "unknown flag: $1" >&2
            print_usage >&2
            exit 1
            ;;
    esac
done

if [ -z "${CASEAGENT_PSQL_DSN:-}" ]; then
    echo "CASEAGENT_PSQL_DSN must be set" >&2
    exit 1
fi

if ! command -v psql >/dev/null 2>&1; then
    echo "psql not found in PATH" >&2
    exit 1
fi

log() {
    printf '[i1-cleanup] %s\n' "$1"
}

PRED="metadata @> '{\"aliases\":[\"I1 smoke fixture\"]}'"

LIST_SQL="SELECT id, type, name, COALESCE(metadata->>'run_token','(no token)') FROM knowledge_base WHERE $PRED ORDER BY id"
DELETE_SQL="DELETE FROM knowledge_base WHERE $PRED RETURNING id"

CANDIDATES="$(psql "$CASEAGENT_PSQL_DSN" -At -F $'\t' -c "$LIST_SQL")"

if [ -z "$CANDIDATES" ]; then
    log "no legacy I1 smoke knowledge rows found"
    exit 0
fi

log "candidates (id<TAB>type<TAB>name<TAB>run_token):"
printf '%s\n' "$CANDIDATES" | sed 's/^/  /'

if [ "$DRY_RUN" = 1 ]; then
    log "dry-run; no rows deleted"
    exit 0
fi

DELETED_IDS="$(psql "$CASEAGENT_PSQL_DSN" -At -c "$DELETE_SQL")"
DELETED_COUNT=0
if [ -n "$DELETED_IDS" ]; then
    DELETED_COUNT=$(printf '%s\n' "$DELETED_IDS" | grep -c '^[0-9]')
fi
log "deleted $DELETED_COUNT knowledge_base row(s)"
