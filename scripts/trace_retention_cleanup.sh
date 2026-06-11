#!/usr/bin/env bash
set -euo pipefail

# Tenant-scoped diagnostic retention cleanup. Defaults to dry-run so operators
# can inspect row and byte estimates before deleting trace details.

BASE_URL="${CASEAGENT_BASE_URL:-http://localhost:40003}"
TENANT_SLUG="${CASEAGENT_TENANT_SLUG:-demo-caseagent}"
RETENTION_DAYS="${CASEAGENT_TRACE_RETENTION_DAYS:-}"
OPERATOR_ID="${CASEAGENT_OPERATOR_ID:-local:trace-retention-script}"
OPERATOR_NAME="${CASEAGENT_OPERATOR_NAME:-Trace Retention Script}"
REASON="${CASEAGENT_RETENTION_REASON:-diagnostic retention cleanup}"
MODE="dry-run"

usage() {
  cat <<'USAGE'
Usage: bash scripts/trace_retention_cleanup.sh [--days N] [--execute] [--reason TEXT]

Environment:
  CASEAGENT_BASE_URL              Backend base URL, default http://localhost:40003
  CASEAGENT_TENANT_SLUG           Tenant slug, default demo-caseagent
  CASEAGENT_TRACE_RETENTION_DAYS  Override server retention.trace_retention_days
  CASEAGENT_OPERATOR_ID           Operator ID header for execute
  CASEAGENT_OPERATOR_NAME         Operator name header for execute
  CASEAGENT_RETENTION_REASON      Reason for execute

Default mode is dry-run. Use --execute only after reviewing the dry-run output.
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --days)
      RETENTION_DAYS="${2:-}"
      shift 2
      ;;
    --execute)
      MODE="execute"
      shift
      ;;
    --reason)
      REASON="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ -n "$RETENTION_DAYS" && ! "$RETENTION_DAYS" =~ ^[1-9][0-9]*$ ]]; then
  echo "--days must be a positive integer" >&2
  exit 2
fi

if [[ "$MODE" == "execute" && -z "${REASON// }" ]]; then
  echo "--reason is required for --execute" >&2
  exit 2
fi

URL="${BASE_URL%/}/api/v1/ops/retention/cleanup"

if [[ "$MODE" == "dry-run" ]]; then
  if [[ -n "$RETENTION_DAYS" ]]; then
    curl --fail --silent --show-error \
      -H "X-Tenant-ID: ${TENANT_SLUG}" \
      --get \
      --data-urlencode "retention_days=${RETENTION_DAYS}" \
      "$URL" | jq .
  else
    curl --fail --silent --show-error \
      -H "X-Tenant-ID: ${TENANT_SLUG}" \
      "$URL" | jq .
  fi
  exit 0
fi

if [[ -n "$RETENTION_DAYS" ]]; then
  PAYLOAD="$(jq -n --arg reason "$REASON" --argjson retention_days "$RETENTION_DAYS" '{reason: $reason, retention_days: $retention_days}')"
else
  PAYLOAD="$(jq -n --arg reason "$REASON" '{reason: $reason}')"
fi

curl --fail --silent --show-error \
  -H "Content-Type: application/json" \
  -H "X-Tenant-ID: ${TENANT_SLUG}" \
  -H "X-Operator-ID: ${OPERATOR_ID}" \
  -H "X-Operator-Name: ${OPERATOR_NAME}" \
  -d "$PAYLOAD" \
  "$URL" | jq .
