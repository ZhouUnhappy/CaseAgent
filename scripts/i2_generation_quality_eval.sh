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
#   CASEAGENT_I2_QUALITY_JSON_REPORT      default docs/regression/i2_generation_quality_eval.json
#   CASEAGENT_I2_QUALITY_HTML_REPORT      default docs/regression/i2_generation_quality_eval.html
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
JSON_REPORT="${CASEAGENT_I2_QUALITY_JSON_REPORT:-${REPORT%.md}.json}"
HTML_REPORT="${CASEAGENT_I2_QUALITY_HTML_REPORT:-${REPORT%.md}.html}"
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

mkdir -p "$(dirname "$REPORT")" "$(dirname "$JSON_REPORT")" "$(dirname "$HTML_REPORT")" "$(dirname "$E2E_REPORT")"

write_quality_html() {
    local html_report="$1"
    local report_json="$2"

    {
        cat <<'HTML'
<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>CaseAgent I2 Quality Report</title>
  <style>
    :root {
      color-scheme: light;
      --bg: #f6f8fb;
      --panel: #ffffff;
      --border: #d8e0ea;
      --ink: #172033;
      --muted: #637083;
      --blue: #2563eb;
      --green: #059669;
      --amber: #d97706;
      --red: #dc2626;
      --track: #e9eef5;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      background: var(--bg);
      color: var(--ink);
      font: 14px/1.5 ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
    }
    main {
      width: min(1180px, calc(100vw - 32px));
      margin: 0 auto;
      padding: 28px 0 40px;
    }
    header {
      display: flex;
      align-items: flex-start;
      justify-content: space-between;
      gap: 18px;
      margin-bottom: 18px;
    }
    h1, h2, h3, p { margin: 0; }
    h1 { font-size: 24px; line-height: 1.2; }
    h2 { font-size: 16px; line-height: 1.3; }
    h3 { font-size: 13px; color: var(--muted); font-weight: 700; }
    .muted { color: var(--muted); }
    .meta, .grid, .charts, .columns { display: grid; gap: 12px; }
    .meta { grid-template-columns: repeat(4, minmax(0, 1fr)); margin-bottom: 14px; }
    .grid { grid-template-columns: repeat(6, minmax(0, 1fr)); margin-bottom: 14px; }
    .charts { grid-template-columns: repeat(2, minmax(0, 1fr)); margin-bottom: 14px; }
    .columns { grid-template-columns: repeat(3, minmax(0, 1fr)); }
    .panel {
      background: var(--panel);
      border: 1px solid var(--border);
      border-radius: 8px;
      padding: 14px;
      min-width: 0;
    }
    .panel h2 { margin-bottom: 12px; }
    .chip-row {
      display: flex;
      align-items: center;
      gap: 8px;
      flex-wrap: wrap;
    }
    .chip {
      display: inline-flex;
      align-items: center;
      min-height: 24px;
      border: 1px solid #cbd5e1;
      border-radius: 999px;
      padding: 0 9px;
      background: #fff;
      color: #334155;
      font-size: 12px;
      font-weight: 700;
    }
    .kpi strong {
      display: block;
      font-size: 24px;
      line-height: 1.1;
    }
    .kpi span {
      display: block;
      margin-top: 5px;
      color: var(--muted);
      font-size: 12px;
      font-weight: 700;
    }
    .ok strong { color: var(--green); }
    .warn strong { color: var(--amber); }
    .bad strong { color: var(--red); }
    .bar-row {
      display: grid;
      grid-template-columns: 150px minmax(0, 1fr) 58px;
      gap: 10px;
      align-items: center;
      margin: 10px 0;
    }
    .track {
      height: 10px;
      overflow: hidden;
      border-radius: 999px;
      background: var(--track);
    }
    .fill {
      height: 100%;
      border-radius: inherit;
      background: var(--blue);
    }
    .fill.ok { background: var(--green); }
    .fill.warn { background: var(--amber); }
    .fill.bad { background: var(--red); }
    table {
      width: 100%;
      border-collapse: collapse;
      font-size: 13px;
    }
    th, td {
      border-bottom: 1px solid #e5eaf1;
      padding: 8px 6px;
      text-align: left;
      vertical-align: top;
    }
    th { color: var(--muted); font-weight: 700; }
    code {
      color: #0f172a;
      font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
      font-size: 12px;
      word-break: break-word;
    }
    .empty {
      color: var(--muted);
      padding: 8px 0;
    }
    @media (max-width: 920px) {
      .meta, .grid, .charts, .columns { grid-template-columns: 1fr; }
      header { flex-direction: column; }
      .bar-row { grid-template-columns: 1fr; }
    }
  </style>
</head>
<body>
<main>
  <header>
    <div>
      <h1>CaseAgent I2 Quality Report</h1>
      <p class="muted" id="subtitle"></p>
    </div>
    <div class="chip-row" id="headerChips"></div>
  </header>

  <section class="meta" id="metaGrid"></section>
  <section class="grid" id="kpiGrid"></section>

  <section class="charts">
    <div class="panel">
      <h2>质量覆盖率</h2>
      <div id="rateBars"></div>
    </div>
    <div class="panel">
      <h2>Trace 分布</h2>
      <div id="traceBars"></div>
    </div>
  </section>

  <section class="charts">
    <div class="panel">
      <h2>Model Call 统计</h2>
      <div id="modelStats"></div>
    </div>
    <div class="panel">
      <h2>失败阶段分布</h2>
      <div id="failureStages"></div>
    </div>
  </section>

  <section class="columns">
    <div class="panel">
      <h2>Prompt Version</h2>
      <div id="promptVersions"></div>
    </div>
    <div class="panel">
      <h2>Model Status</h2>
      <div id="modelStatuses"></div>
    </div>
    <div class="panel">
      <h2>失败原因</h2>
      <div id="failureReasons"></div>
    </div>
  </section>
</main>
<script type="application/json" id="report-json">
HTML
        jq '.' <<<"$report_json"
        cat <<'HTML'
</script>
<script>
  const report = JSON.parse(document.getElementById('report-json').textContent);
  const metrics = report.metrics || report;
  const thresholds = report.thresholds || {};

  const fmtRate = (value) => `${Math.round((Number(value) || 0) * 1000) / 10}%`;
  const fmtNumber = (value) => new Intl.NumberFormat().format(Number(value) || 0);
  const byId = (id) => document.getElementById(id);
  const escapeHtml = (value) => String(value ?? '').replace(/[&<>"']/g, (ch) => ({
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    '"': '&quot;',
    "'": '&#39;',
  }[ch]));

  function statusClass(label, ok) {
    if (ok === true) return 'ok';
    if (ok === false) return 'bad';
    return label;
  }

  function card(label, value, cls = '') {
    return `<div class="panel kpi ${cls}"><strong>${escapeHtml(value)}</strong><span>${escapeHtml(label)}</span></div>`;
  }

  function bar(label, value, max = 1, cls = '', isRate = false) {
    const numeric = Number(value) || 0;
    const denom = Math.max(Number(max) || 1, numeric, 1);
    const pct = Math.max(0, Math.min(100, (numeric / denom) * 100));
    const shown = isRate ? fmtRate(numeric) : fmtNumber(numeric);
    return `
      <div class="bar-row">
        <strong>${escapeHtml(label)}</strong>
        <div class="track"><div class="fill ${escapeHtml(cls)}" style="width:${pct}%"></div></div>
        <span class="muted">${escapeHtml(shown)}</span>
      </div>`;
  }

  function table(rows, columns) {
    if (!rows || rows.length === 0) return '<div class="empty">暂无数据</div>';
    const head = `<thead><tr>${columns.map((col) => `<th>${escapeHtml(col.label)}</th>`).join('')}</tr></thead>`;
    const body = `<tbody>${rows.map((row) => `
      <tr>${columns.map((col) => `<td>${col.code ? `<code>${escapeHtml(row[col.key])}</code>` : escapeHtml(row[col.key])}</td>`).join('')}</tr>`).join('')}</tbody>`;
    return `<table>${head}${body}</table>`;
  }

  byId('subtitle').textContent = `generated ${report.generated_at || '-'} · task #${metrics.task_id || '-'}`;
  byId('headerChips').innerHTML = [
    ['tenant', report.tenant],
    ['status', metrics.terminal_status],
    ['profile', `${metrics.generation_profile_id || '-'}@${metrics.generation_profile_version || '-'}`],
    ['base', report.base_url],
  ].map(([label, value]) => `<span class="chip">${escapeHtml(label)}: ${escapeHtml(value || '-')}</span>`).join('');

  byId('metaGrid').innerHTML = [
    card('E2E report', report.e2e_report || '-', ''),
    card('generation profile', `${metrics.generation_profile_id || '-'}@${metrics.generation_profile_version || '-'}`, ''),
    card('duplicate max', thresholds.duplicate_title_count_max ?? '-', ''),
    card('field min', thresholds.field_complete_rate_min ?? '-', ''),
  ].join('');

  const fieldOk = Number(metrics.field_complete_rate) >= Number(thresholds.field_complete_rate_min ?? 0);
  const sourceOk = Number(metrics.source_context_coverage) >= Number(thresholds.source_context_coverage_min ?? 0);
  const duplicateOk = Number(metrics.duplicate_title_count) <= Number(thresholds.duplicate_title_count_max ?? 0);
  const modelOk = Number(metrics.model_call_count) >= Number(thresholds.model_call_count_min ?? 0);
  byId('kpiGrid').innerHTML = [
    card('sections', fmtNumber(metrics.section_count)),
    card('cases', fmtNumber(metrics.case_count)),
    card('duplicate titles', fmtNumber(metrics.duplicate_title_count), statusClass('', duplicateOk)),
    card('field complete', fmtRate(metrics.field_complete_rate), statusClass('', fieldOk)),
    card('source context', fmtRate(metrics.source_context_coverage), statusClass('', sourceOk)),
    card('model calls', fmtNumber(metrics.model_call_count), statusClass('', modelOk)),
  ].join('');

  byId('rateBars').innerHTML = [
    bar('field complete', metrics.field_complete_rate, 1, fieldOk ? 'ok' : 'bad', true),
    bar('source context', metrics.source_context_coverage, 1, sourceOk ? 'ok' : 'bad', true),
    bar('product hit', metrics.product_hit_rate, 1, 'ok', true),
    bar('module hit', metrics.module_hit_rate, 1, 'ok', true),
  ].join('');

  const trace = metrics.trace_counts || {};
  const traceRows = Object.entries(trace);
  const traceMax = Math.max(...traceRows.map(([, value]) => Number(value) || 0), 1);
  byId('traceBars').innerHTML = traceRows.map(([label, value]) => bar(label, value, traceMax, 'ok')).join('');

  const usage = metrics.model_call_usage || {};
  byId('modelStats').innerHTML = [
    bar('prompt chars', metrics.model_call_prompt_chars, Math.max(metrics.model_call_prompt_chars, metrics.model_call_response_chars, 1), 'ok'),
    bar('response chars', metrics.model_call_response_chars, Math.max(metrics.model_call_prompt_chars, metrics.model_call_response_chars, 1), 'ok'),
    bar('prompt tokens', usage.prompt_tokens, Math.max(usage.prompt_tokens, usage.completion_tokens, usage.total_tokens, 1), 'ok'),
    bar('completion tokens', usage.completion_tokens, Math.max(usage.prompt_tokens, usage.completion_tokens, usage.total_tokens, 1), 'ok'),
    bar('total tokens', usage.total_tokens, Math.max(usage.prompt_tokens, usage.completion_tokens, usage.total_tokens, 1), 'ok'),
  ].join('');

  const failureStages = metrics.failure_stage_distribution || [];
  const failureMax = Math.max(...failureStages.map((row) => Number(row.count) || 0), 1);
  byId('failureStages').innerHTML = failureStages.length
    ? failureStages.map((row) => bar(row.stage, row.count, failureMax, 'bad')).join('')
    : '<div class="empty">暂无失败阶段</div>';

  byId('promptVersions').innerHTML = table(metrics.prompt_version_distribution || [], [
    { key: 'prompt_id', label: 'prompt_id', code: true },
    { key: 'prompt_version', label: 'version', code: true },
    { key: 'count', label: 'count' },
  ]);

  byId('modelStatuses').innerHTML = table(metrics.model_call_status_counts || [], [
    { key: 'status', label: 'status', code: true },
    { key: 'count', label: 'count' },
  ]);

  byId('failureReasons').innerHTML = table(metrics.failure_reason_distribution || [], [
    { key: 'reason', label: 'reason', code: true },
    { key: 'count', label: 'count' },
  ]);
</script>
</body>
</html>
HTML
    } > "$html_report"
}

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
    | (($cases[0].source_context.generation_profile // {}) ) as $generation_profile
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
        generation_profile: $generation_profile,
        generation_profile_id: ($generation_profile.id // $cases[0].source_context.generation_profile_id // "unknown"),
        generation_profile_version: ($generation_profile.version // $cases[0].source_context.generation_profile_version // "unknown"),
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
GENERATED_AT="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"

report_json="$(jq -n \
    --arg generated_at "$GENERATED_AT" \
    --arg base_url "$BASE_URL" \
    --arg tenant "$TENANT_SLUG" \
    --arg e2e_report "$E2E_REPORT" \
    --arg duplicate_title_max "$DUPLICATE_TITLE_MAX" \
    --arg field_complete_min "$FIELD_COMPLETE_MIN" \
    --arg source_context_min "$SOURCE_CONTEXT_MIN" \
    --arg model_call_min "$MODEL_CALL_MIN" \
    --argjson metrics "$metrics" \
    '{
      generated_at: $generated_at,
      base_url: $base_url,
      tenant: $tenant,
      e2e_report: $e2e_report,
      thresholds: {
        duplicate_title_count_max: ($duplicate_title_max | tonumber),
        field_complete_rate_min: ($field_complete_min | tonumber),
        source_context_coverage_min: ($source_context_min | tonumber),
        model_call_count_min: ($model_call_min | tonumber)
      },
      metrics: $metrics
    }'
)"

jq '.' <<<"$report_json" > "$JSON_REPORT"
write_quality_html "$HTML_REPORT" "$report_json"

{
    printf '# I2 Generation Quality Eval\n\n'
    printf -- '- base_url: `%s`\n' "$BASE_URL"
    printf -- '- tenant: `%s`\n' "$TENANT_SLUG"
    printf -- '- generated_at: `%s`\n' "$GENERATED_AT"
    printf -- '- e2e_report: `%s`\n' "$E2E_REPORT"
    printf -- '- json_report: `%s`\n' "$JSON_REPORT"
    printf -- '- html_report: `%s`\n' "$HTML_REPORT"
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

echo "[i2-quality] passed (report: $REPORT, json: $JSON_REPORT, html: $HTML_REPORT)"
