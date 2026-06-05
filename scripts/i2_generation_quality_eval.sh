#!/usr/bin/env bash
# I2 quality eval: run the fixed public-corpus generation path and record
# structured quality metrics with explicit thresholds.
#
# Default tenant: apache-dubbo (same public fixture tenant as i1_public_corpus).
#
# Env:
#   CASEAGENT_BASE_URL                    default http://localhost:40003/api/v1
#   CASEAGENT_PSQL_DSN                    required by i2_generation_e2e.sh
#   CASEAGENT_TENANT_SLUG                 default apache-dubbo
#   CASEAGENT_I2_QUALITY_REPORT           default docs/regression/i2_generation_quality_eval.md
#   CASEAGENT_I2_QUALITY_E2E_REPORT       default .dev/i2_generation_quality_e2e.md
#   CASEAGENT_I2_FIELD_COMPLETE_MIN       default 1.0
#   CASEAGENT_I2_SOURCE_CONTEXT_MIN       default 1.0
#   CASEAGENT_I2_DUPLICATE_TITLE_MAX      default 0
#   CASEAGENT_I2_MODEL_CALL_MIN           default 1

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASE_URL="${CASEAGENT_BASE_URL:-http://localhost:40003/api/v1}"
TENANT_SLUG="${CASEAGENT_TENANT_SLUG:-apache-dubbo}"
REPORT="${CASEAGENT_I2_QUALITY_REPORT:-$ROOT_DIR/docs/regression/i2_generation_quality_eval.md}"
E2E_REPORT="${CASEAGENT_I2_QUALITY_E2E_REPORT:-$ROOT_DIR/.dev/i2_generation_quality_e2e.md}"
FIELD_COMPLETE_MIN="${CASEAGENT_I2_FIELD_COMPLETE_MIN:-1.0}"
SOURCE_CONTEXT_MIN="${CASEAGENT_I2_SOURCE_CONTEXT_MIN:-1.0}"
DUPLICATE_TITLE_MAX="${CASEAGENT_I2_DUPLICATE_TITLE_MAX:-0}"
MODEL_CALL_MIN="${CASEAGENT_I2_MODEL_CALL_MIN:-1}"

require_command() {
    if ! command -v "$1" >/dev/null 2>&1; then
        echo "missing required command: $1" >&2
        exit 1
    fi
}
require_command bash
require_command curl
require_command jq
require_command awk

mkdir -p "$(dirname "$REPORT")" "$(dirname "$E2E_REPORT")"

CASEAGENT_I2_E2E_REPORT="$E2E_REPORT" \
CASEAGENT_TENANT_SLUG="$TENANT_SLUG" \
bash "$ROOT_DIR/scripts/i2_generation_e2e.sh"

TASK_ID="$(awk -F'`' '/task_id:/ {print $2; exit}' "$E2E_REPORT")"
if [ -z "$TASK_ID" ]; then
    echo "failed to resolve task_id from $E2E_REPORT" >&2
    exit 1
fi

get_json() {
    curl --fail --silent --show-error --noproxy '*' \
        -H "X-Tenant-ID: $TENANT_SLUG" \
        "$BASE_URL$1"
}

cases_response="$(get_json "/tasks/$TASK_ID/cases")"
jobs_response="$(get_json "/jobs?task_id=$TASK_ID")"
task_response="$(get_json "/tasks/$TASK_ID")"
trace_response="$(get_json "/tasks/$TASK_ID/trace")"

metrics="$(jq -n \
    --argjson cases "$cases_response" \
    --argjson jobs "$jobs_response" \
    --argjson task "$task_response" \
    --argjson trace "$trace_response" '
    def all_cases: [$cases[] | (.cases // [])[]];
    def rate($num; $den): if $den == 0 then 0 else (($num * 1000 / $den) | floor / 1000) end;
    def number_or_zero: if type == "number" then . else 0 end;
    def usage_value($usage; $snake; $camel):
      if ($usage | type) == "object" then
        (($usage[$snake] // $usage[$camel] // 0) | number_or_zero)
      else 0 end;
    def status_counts($rows):
      ($rows | group_by(.status // "unknown")
        | map({status: (.[0].status // "unknown"), count: length}));
    def stage_from_error:
      if . == null or . == "" then empty
      else (try capture("^(?<stage>[A-Za-z0-9_\\-]+):").stage catch "unknown") end;

    (all_cases) as $case_rows
    | ($cases | length) as $section_count
    | ($case_rows | length) as $case_count
    | ([ $case_rows[].title ] | group_by(.) | map(select(length > 1)) | length) as $duplicate_title_count
    | ([
        $case_rows[]
        | select(
            ((.title // "") | length) > 0
            and (.priority_id != null)
            and ((.custom_preconds // "") | length) > 0
            and ((.custom_steps_separated // []) | type == "array")
            and ((.custom_steps_separated // []) | length) > 0
        )
      ] | length) as $field_complete_count
    | ([ $case_rows[] | select(((.affected_products // []) | length) > 0) ] | length) as $product_hit_count
    | ([ $case_rows[] | select(((.affected_modules // []) | length) > 0) ] | length) as $module_hit_count
    | ([ $cases[] | select(.source_context != null and (.source_context | type) == "object") ] | length) as $sections_with_source_context
    | ($trace.model_calls // []) as $model_calls
    | ([ $model_calls[] | (.prompt_chars // 0) ] | add // 0) as $model_prompt_chars
    | ([ $model_calls[] | (.response_chars // 0) ] | add // 0) as $model_response_chars
    | ([ $model_calls[] | usage_value(.metadata.usage; "prompt_tokens"; "PromptTokens") ] | add // 0) as $prompt_tokens
    | ([ $model_calls[] | usage_value(.metadata.usage; "completion_tokens"; "CompletionTokens") ] | add // 0) as $completion_tokens
    | ([ $model_calls[] | usage_value(.metadata.usage; "total_tokens"; "TotalTokens") ] | add // 0) as $total_tokens
    | ([
        ($jobs[]? | (.last_error // empty)),
        ($trace.workflow_runs[]? | (.last_error // empty)),
        ($trace.agent_runs[]? | (.last_error // empty)),
        ($trace.model_calls[]? | (.last_error // empty))
      ] | map(select(. != ""))) as $failure_errors
    | ($failure_errors | map(stage_from_error) | group_by(.) | map({stage: .[0], count: length})) as $failure_stages
    | ($failure_errors | group_by(.) | map({reason: .[0], count: length})) as $failure_reasons
    | {
        task_id: $task.id,
        terminal_status: $task.status,
        section_count: $section_count,
        case_count: $case_count,
        duplicate_title_count: $duplicate_title_count,
        field_complete_rate: rate($field_complete_count; $case_count),
        product_hit_rate: rate($product_hit_count; $case_count),
        module_hit_rate: rate($module_hit_count; $case_count),
        source_context_coverage: rate($sections_with_source_context; $section_count),
        trace_counts: {
          workflow_runs: (($trace.workflow_runs // []) | length),
          steps: (($trace.steps // []) | length),
          agent_runs: (($trace.agent_runs // []) | length),
          model_calls: ($model_calls | length),
          retrieval_runs: (($trace.retrieval_runs // []) | length),
          artifacts: (($trace.artifacts // []) | length)
        },
        model_call_count: ($model_calls | length),
        model_call_status_counts: status_counts($model_calls),
        model_call_prompt_chars: $model_prompt_chars,
        model_call_response_chars: $model_response_chars,
        model_call_usage: {
          prompt_tokens: $prompt_tokens,
          completion_tokens: $completion_tokens,
          total_tokens: $total_tokens
        },
        prompt_version_distribution: (
          [ $model_calls[]
            | {
                prompt_id: (.metadata.prompt_id // "unknown"),
                prompt_version: (.metadata.prompt_version // "unknown")
              }
          ]
          | group_by(.prompt_id + "@" + .prompt_version)
          | map({prompt_id: .[0].prompt_id, prompt_version: .[0].prompt_version, count: length})
        ),
        failure_stage_distribution: $failure_stages,
        failure_reason_distribution: $failure_reasons
      }
')"

duplicate_title_count="$(jq -r '.duplicate_title_count' <<<"$metrics")"
field_complete_rate="$(jq -r '.field_complete_rate' <<<"$metrics")"
source_context_coverage="$(jq -r '.source_context_coverage' <<<"$metrics")"
model_call_count="$(jq -r '.model_call_count' <<<"$metrics")"

{
    printf '# I2 Generation Quality Eval\n\n'
    printf -- '- base_url: `%s`\n' "$BASE_URL"
    printf -- '- tenant: `%s`\n' "$TENANT_SLUG"
    printf -- '- e2e_report: `%s`\n' "$E2E_REPORT"
    printf -- '- thresholds:\n'
    printf '  - duplicate_title_count <= `%s`\n' "$DUPLICATE_TITLE_MAX"
    printf '  - field_complete_rate >= `%s`\n' "$FIELD_COMPLETE_MIN"
    printf '  - source_context_coverage >= `%s`\n' "$SOURCE_CONTEXT_MIN"
    printf '  - model_call_count >= `%s`\n' "$MODEL_CALL_MIN"
    printf '\n## Metrics\n\n'
    printf '```json\n%s\n```\n' "$(jq '.' <<<"$metrics")"
} > "$REPORT"

cat "$REPORT"

awk -v got="$duplicate_title_count" -v max="$DUPLICATE_TITLE_MAX" 'BEGIN { exit !(got <= max) }' || {
    echo "quality threshold failed: duplicate_title_count=$duplicate_title_count max=$DUPLICATE_TITLE_MAX" >&2
    exit 1
}
awk -v got="$field_complete_rate" -v min="$FIELD_COMPLETE_MIN" 'BEGIN { exit !(got >= min) }' || {
    echo "quality threshold failed: field_complete_rate=$field_complete_rate min=$FIELD_COMPLETE_MIN" >&2
    exit 1
}
awk -v got="$source_context_coverage" -v min="$SOURCE_CONTEXT_MIN" 'BEGIN { exit !(got >= min) }' || {
    echo "quality threshold failed: source_context_coverage=$source_context_coverage min=$SOURCE_CONTEXT_MIN" >&2
    exit 1
}
awk -v got="$model_call_count" -v min="$MODEL_CALL_MIN" 'BEGIN { exit !(got >= min) }' || {
    echo "quality threshold failed: model_call_count=$model_call_count min=$MODEL_CALL_MIN" >&2
    exit 1
}

echo "[i2-quality] passed (report: $REPORT)"
