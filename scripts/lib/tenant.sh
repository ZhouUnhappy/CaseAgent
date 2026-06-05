#!/usr/bin/env bash
# Shared tenant helpers for scripts/i*.sh.
#
# Source after BASE_URL is set:
#
#   BASE_URL="${CASEAGENT_BASE_URL:-http://localhost:40003/api/v1}"
#   # shellcheck source=lib/tenant.sh
#   . "$ROOT_DIR/scripts/lib/tenant.sh"
#
# All curl helpers expect TENANT_SLUG to be set before they're called; each
# script picks its own default per the tenant table in scripts/README.md.

ensure_tenant() {
    local slug="$1"
    local name="${2:-$slug}"

    if curl --fail --silent --show-error --noproxy '*' "$BASE_URL/tenants" \
        | jq -e --arg slug "$slug" '.[] | select(.slug == $slug)' >/dev/null 2>&1; then
        return 0
    fi

    curl --fail --silent --show-error --noproxy '*' \
        -H 'Content-Type: application/json' \
        -X POST \
        -d "$(jq -n --arg slug "$slug" --arg name "$name" '{slug: $slug, name: $name}')" \
        "$BASE_URL/tenants" >/dev/null
}

sql_escape_literal() {
    printf "%s" "$1" | sed "s/'/''/g"
}

tenant_id_for_slug() {
    local slug
    slug="$(sql_escape_literal "$1")"
    psql "$CASEAGENT_PSQL_DSN" -v ON_ERROR_STOP=1 -qAt -c \
        "SELECT id FROM tenants WHERE slug = '$slug'"
}

psql_tenant() {
    local slug="$1"
    local sql="$2"
    shift 2

    local tenant_id
    tenant_id="$(tenant_id_for_slug "$slug")"
    if [ -z "$tenant_id" ]; then
        echo "tenant '$slug' not found" >&2
        return 1
    fi

    psql "$CASEAGENT_PSQL_DSN" -v ON_ERROR_STOP=1 -qAt "$@" -c \
        "BEGIN; SET LOCAL app.tenant_id = '$tenant_id'; $sql; COMMIT;"
}

# tenant_slug_for_document <doc_id>: look up tenant slug via psql for scripts
# that reuse existing run data (determinism, i2 e2e). Requires CASEAGENT_PSQL_DSN
# pointing to either a RLS-bypassing role or the tenant app role plus
# CASEAGENT_TENANT_SLUG.
tenant_slug_for_document() {
    local doc_id="$1"
    if [ -n "${CASEAGENT_TENANT_SLUG:-}" ]; then
        if [ -n "$(psql_tenant "$CASEAGENT_TENANT_SLUG" "SELECT id FROM documents WHERE id = $doc_id")" ]; then
            printf "%s" "$CASEAGENT_TENANT_SLUG"
        fi
        return
    fi

    psql "$CASEAGENT_PSQL_DSN" -At -c \
        "SELECT t.slug FROM tenants t JOIN documents d ON d.tenant_id = t.id WHERE d.id = $doc_id"
}

tenant_slug_for_project() {
    local project_id="$1"
    if [ -n "${CASEAGENT_TENANT_SLUG:-}" ]; then
        if [ -n "$(psql_tenant "$CASEAGENT_TENANT_SLUG" "SELECT id FROM projects WHERE id = $project_id")" ]; then
            printf "%s" "$CASEAGENT_TENANT_SLUG"
        fi
        return
    fi

    psql "$CASEAGENT_PSQL_DSN" -At -c \
        "SELECT t.slug FROM tenants t JOIN projects p ON p.tenant_id = t.id WHERE p.id = $project_id"
}
