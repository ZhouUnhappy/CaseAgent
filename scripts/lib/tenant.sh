#!/usr/bin/env bash
# Shared tenant helpers for scripts/i*.sh.
#
# Source after BASE_URL is set:
#
#   BASE_URL="${CASEAGENT_BASE_URL:-http://localhost:8080/api/v1}"
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

# tenant_slug_for_document <doc_id>: look up tenant slug via psql for scripts
# that reuse existing run data (determinism, i2 e2e). Requires CASEAGENT_PSQL_DSN
# pointing to a role that can read tenants/documents (RLS-bypassing OK here
# since this is a lookup, not a tenant-scoped query).
tenant_slug_for_document() {
    local doc_id="$1"
    psql "$CASEAGENT_PSQL_DSN" -At -c \
        "SELECT t.slug FROM tenants t JOIN documents d ON d.tenant_id = t.id WHERE d.id = $doc_id"
}

tenant_slug_for_project() {
    local project_id="$1"
    psql "$CASEAGENT_PSQL_DSN" -At -c \
        "SELECT t.slug FROM tenants t JOIN projects p ON p.tenant_id = t.id WHERE p.id = $project_id"
}
