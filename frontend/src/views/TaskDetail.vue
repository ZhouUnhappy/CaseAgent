<script setup>
import { computed, nextTick, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { storeToRefs } from 'pinia'
import { ElMessageBox } from 'element-plus'
import {
  ArrowDownBold,
  ArrowUpBold,
  ChatDotRound,
  Check,
  Delete,
  Download,
  EditPen,
  Filter,
  Plus,
  Refresh,
  Search,
  View,
  Warning,
} from '@element-plus/icons-vue'
import StatusTag from '../components/StatusTag.vue'
import { listJobs } from '../api/jobs'
import { getTaskDiagnostics, getTaskTrace } from '../api/tasks'
import { useTasksStore } from '../stores/tasks'
import { useTestCasesStore } from '../stores/testcases'
import { useKnowledgeStore } from '../stores/knowledge'
import { useKnowledgeSuggestionsStore } from '../stores/knowledgeSuggestions'
import { useTenantStore } from '../stores/tenant'
import { formatDateTime as formatDate } from '../utils/date'
import { notifySuccess } from '../utils/error'
import { compactJobError, jobStatusLabel, jobStatusType, jobTypeLabel } from '../utils/jobs'
import { knowledgeTypeLabel } from '../utils/labels'

const route = useRoute()
const router = useRouter()
const taskId = computed(() => Number(route.params.id))

const tasksStore = useTasksStore()
const casesStore = useTestCasesStore()
const knowledgeStore = useKnowledgeStore()
const suggestionStore = useKnowledgeSuggestionsStore()
const tenantStore = useTenantStore()

const { current: task } = storeToRefs(tasksStore)
const {
  items: cases,
  loading: casesLoading,
  saving: casesSaving,
  feedbackSaving,
  batchSaving,
} = storeToRefs(casesStore)
const { items: knowledge } = storeToRefs(knowledgeStore)

const reviewForm = reactive({ products: [], modules: [] })
const polling = ref(null)
const generating = ref(false)
const retrying = ref(false)
const jobs = ref([])
const jobsLoading = ref(false)
const trace = ref(null)
const traceLoading = ref(false)
const diagnosticsDownloading = ref(false)
const editingCase = ref(null)
const editorVisible = ref(false)
const editorBuffer = ref('')
const editorOriginal = ref('')
const provenanceVisible = ref(false)
const provenanceCase = ref(null)
const selectedCaseRefs = ref({})
const selectedSectionIds = ref([])
const expandedSections = ref([])
const expandedCaseRows = ref({})
const overviewExpanded = ref(true)
const advancedFiltersVisible = ref(false)
const focusedCaseKey = ref('')
const reviewViewReady = ref(false)
const restoredFocusApplied = ref(false)
const activeTaskView = ref('review')
const provenanceTab = ref('sources')
const caseEditorVisible = ref(false)
const editingCaseSection = ref(null)
const editingCaseIndex = ref(-1)
const caseEditorOriginal = ref('')
const caseForm = reactive({
  title: '',
  priority_id: 2,
  custom_preconds: '',
  affected_products: [],
  affected_modules: [],
  custom_steps_separated: [],
})
const caseFilters = reactive({
  keyword: '',
  section: '',
  priority_id: '',
  product: '',
  module: '',
  feedback_type: '',
  review_status: '',
  provenance: '',
  hide_duplicates: true,
})
const batchPatch = reactive({
  priority_id: '',
  affected_products: [],
  affected_modules: [],
})
const qualityFeedbackVisible = ref(false)
const qualityFeedbackForm = reactive({
  test_case_id: 0,
  case_index: 0,
  case_title: '',
  feedback_type: 'useful',
  note: '',
})
const feedbackVisible = ref(false)
const feedbackForm = reactive({
  candidate_type: 'module',
  candidate_name: '',
  source_case_id: 0,
  source_case_index: 0,
  source_task_id: 0,
  source_case_title: '',
  note: '',
})
const qualityFeedbackOptions = [
  { value: 'useful', label: '有用' },
  { value: 'duplicate', label: '重复' },
  { value: 'missing_steps', label: '缺步骤' },
  { value: 'requirement_mismatch', label: '不符合需求' },
  { value: 'knowledge_missing', label: '知识缺失' },
]
const priorityOptions = [
  { value: 1, label: 'Low' },
  { value: 2, label: 'Medium' },
  { value: 3, label: 'High' },
  { value: 4, label: 'Critical' },
]

const productOptions = computed(() =>
  knowledge.value.filter((k) => k.type === 'product').map((k) => k.name),
)
const moduleOptions = computed(() =>
  knowledge.value.filter((k) => k.type === 'module').map((k) => k.name),
)

const canReview = computed(() =>
  task.value && ['awaiting_review', 'ready_to_generate'].includes(task.value.status),
)
const canGenerate = computed(() => task.value?.status === 'ready_to_generate')
const isPolling = computed(() => task.value && ['analyzing', 'generating'].includes(task.value.status))
const caseSectionCount = computed(() => cases.value.length)
const totalCaseCount = computed(() =>
  cases.value.reduce((sum, section) => sum + (section.cases?.length || 0), 0),
)
const sectionOptions = computed(() => cases.value.map((section) => section.section).filter(Boolean))
const selectedCases = computed(() => Object.values(selectedCaseRefs.value).flat())
const selectedCaseCount = computed(() => selectedCases.value.length)
const selectedSectionCount = computed(() => selectedSectionIds.value.length)
const latestFeedbackByCase = computed(() => {
  const result = new Map()
  for (const item of trace.value?.feedback || []) {
    const key = `${item.test_case_id}:${item.case_index}`
    if (!result.has(key)) result.set(key, item)
  }
  return result
})
const reviewedCaseCount = computed(() =>
  cases.value.reduce(
    (sum, section) => sum + (section.cases || []).filter((row, index) => caseReviewState(section, row, index) !== 'pending').length,
    0,
  ),
)
const passedCaseCount = computed(() =>
  cases.value.reduce(
    (sum, section) => sum + (section.cases || []).filter((row, index) => caseReviewState(section, row, index) === 'passed').length,
    0,
  ),
)
const resolvedCaseCount = computed(() =>
  cases.value.reduce(
    (sum, section) => sum + (section.cases || []).filter((row, index) => caseReviewState(section, row, index) === 'resolved').length,
    0,
  ),
)
const issueCaseCount = computed(() =>
  cases.value.reduce(
    (sum, section) => sum + (section.cases || []).filter((row, index) => caseReviewState(section, row, index) === 'issue').length,
    0,
  ),
)
const hasAdvancedCaseFilters = computed(() =>
  Boolean(
    caseFilters.priority_id ||
      caseFilters.product ||
      caseFilters.module ||
      caseFilters.feedback_type ||
      caseFilters.provenance,
  ),
)
const hasActiveCaseFilters = computed(() =>
  Boolean(
    caseFilters.section ||
      caseFilters.keyword ||
      caseFilters.priority_id ||
      caseFilters.product ||
      caseFilters.module ||
      caseFilters.feedback_type ||
      caseFilters.review_status ||
      caseFilters.provenance ||
      caseFilters.hide_duplicates,
  ),
)
const filteredSections = computed(() =>
  cases.value
    .map((section) => {
      const displayCases = (section.cases || [])
        .map((row, index) => ({ ...row, __case_index: index }))
        .filter((row) => matchesCaseFilters(section, row, row.__case_index))
      return { ...section, display_cases: displayCases }
    })
    .filter((section) => section.display_cases.length > 0 || !hasActiveCaseFilters.value),
)
const filteredCaseCount = computed(() =>
  filteredSections.value.reduce((sum, section) => sum + section.display_cases.length, 0),
)
const allCaseDetailsExpanded = computed(() =>
  filteredSections.value.length > 0 &&
  filteredSections.value.every(
    (section) => isSectionExpanded(section) && areSectionCasesExpanded(section),
  ),
)
const editorDirty = computed(() => editorVisible.value && editorBuffer.value !== editorOriginal.value)
const caseEditorDirty = computed(() => caseEditorVisible.value && serializeCaseForm() !== caseEditorOriginal.value)
const caseOutputEmptyDescription = computed(() => {
  switch (task.value?.status) {
    case 'analyzing':
      return '正在分析影响范围'
    case 'awaiting_review':
      return '等待影响范围审核'
    case 'ready_to_generate':
      return '等待开始生成'
    case 'generating':
      return '正在生成测试用例'
    case 'failed':
      return '暂无已保存用例'
    case 'completed':
      return '暂无用例'
    default:
      return '暂无用例'
  }
})
const jobTimeline = computed(() => {
  if (!task.value) return []

  const rows = [
    {
      key: 'created',
      label: '任务已创建',
      status: 'succeeded',
      time: task.value.created_at,
    },
  ]

  for (const job of jobs.value) {
    rows.push({
      key: `job-${job.id}`,
      label: jobTypeLabel(job.job_type),
      status: job.status,
      time: job.started_at || job.run_after || job.created_at,
      retry: `${job.retry_count}/${job.max_retries}`,
      error: compactJobError(job),
      nextRun: job.status === 'retrying' ? job.run_after : '',
    })
  }

  if (['awaiting_review', 'ready_to_generate'].includes(task.value.status)) {
    rows.push({
      key: 'review',
      label: '影响范围确认',
      status: task.value.status === 'awaiting_review' ? 'running' : 'succeeded',
      time: task.value.updated_at,
    })
  }
  if (['completed', 'failed'].includes(task.value.status)) {
    rows.push({
      key: 'final',
      label: task.value.status === 'completed' ? '任务已完成' : '任务失败',
      status: task.value.status === 'completed' ? 'succeeded' : 'failed',
      time: task.value.updated_at,
      error: lastJobError.value,
    })
  }
  return rows
})
const lastJobError = computed(() => compactJobError(findLastJobWithError()))
const traceSummary = computed(() => {
  const value = trace.value || {}
  return {
    workflows: value.workflow_runs?.length || 0,
    steps: value.steps?.length || 0,
    agents: value.agent_runs?.length || 0,
    modelCalls: value.model_calls?.length || 0,
    retrievals: value.retrieval_runs?.length || 0,
    artifacts: value.artifacts?.length || 0,
    feedback: feedbackCountTotal(value.feedback_summary),
    lastError: latestTraceError(value),
  }
})
const traceRuns = computed(() => trace.value?.workflow_runs || [])
const traceAgents = computed(() => trace.value?.agent_runs || [])
const traceModelCalls = computed(() => trace.value?.model_calls || [])
const traceRetrievals = computed(() => trace.value?.retrieval_runs || [])
const traceArtifacts = computed(() => trace.value?.artifacts || [])
const traceCaseProvenance = computed(() => trace.value?.case_provenance || [])
const hasTraceData = computed(() =>
  Boolean(
    traceSummary.value.workflows ||
      traceSummary.value.steps ||
      traceSummary.value.agents ||
      traceSummary.value.modelCalls ||
      traceSummary.value.retrievals ||
      traceSummary.value.artifacts ||
      traceSummary.value.feedback,
  ),
)
const traceCostSummary = computed(() => {
  const calls = traceModelCalls.value
  return {
    tokens: calls.reduce((sum, call) => sum + modelCallTokens(call), 0),
    fallbacks: calls.filter((call) => call.metadata?.provider_role === 'fallback').length,
    guardrails: calls.filter((call) => call.metadata?.guardrail_event).length,
    rateLimits: calls.filter((call) => String(call.last_error || '').toLowerCase().includes('rate limit')).length,
  }
})

onMounted(async () => {
  restoreReviewViewState()
  reviewViewReady.value = true
  await loadTask()
  knowledgeStore.fetch().catch(() => {})
})
onUnmounted(() => stopPolling())

watch(taskId, () => {
  stopPolling()
  reviewViewReady.value = false
  restoredFocusApplied.value = false
  casesStore.clear()
  clearSelectedCases()
  selectedSectionIds.value = []
  resetCaseFilters()
  overviewExpanded.value = true
  advancedFiltersVisible.value = false
  focusedCaseKey.value = ''
  clearCaseExpansion()
  restoreReviewViewState()
  reviewViewReady.value = true
  loadTask()
})

watch(
  [
    overviewExpanded,
    advancedFiltersVisible,
    expandedSections,
    expandedCaseRows,
    focusedCaseKey,
    () => ({ ...caseFilters }),
  ],
  () => persistReviewViewState(),
  { deep: true },
)

watch(
  () => task.value?.status,
  (status) => {
    if (status === 'completed') {
      refreshCases().catch(() => {})
      stopPolling()
    } else if (status === 'failed') {
      refreshCases().catch(() => {})
      stopPolling()
    } else if (status === 'analyzing' || status === 'generating') {
      casesStore.clear()
      ensurePolling()
    } else {
      casesStore.clear()
      stopPolling()
    }
  },
)

async function loadTask() {
  try {
    const t = await tasksStore.load(taskId.value)
    loadJobs().catch(() => {})
    loadTrace().catch(() => {})
    reviewForm.products = [...(t.affected_products || [])]
    reviewForm.modules = [...(t.affected_modules || [])]
    if (['completed', 'failed'].includes(t.status)) {
      await refreshCases()
    } else {
      casesStore.clear()
    }
  } catch {
    /* 错误已弹窗 */
  }
}

async function loadJobs() {
  jobsLoading.value = true
  try {
    jobs.value = await listJobs({ task_id: taskId.value })
  } finally {
    jobsLoading.value = false
  }
}

async function loadTrace() {
  traceLoading.value = true
  try {
    trace.value = await getTaskTrace(taskId.value)
  } finally {
    traceLoading.value = false
  }
}

async function refreshCases() {
  await casesStore.fetch(taskId.value)
  clearSelectedCases()
  selectedSectionIds.value = selectedSectionIds.value.filter((id) =>
    cases.value.some((section) => section.id === id && !['submitted', 'approved'].includes(section.status)),
  )
  pruneReviewViewState()
  await restoreFocusedCase()
}

function ensurePolling() {
  if (polling.value) return
  polling.value = setInterval(() => {
    tasksStore.load(taskId.value).catch(() => {})
    loadJobs().catch(() => {})
    loadTrace().catch(() => {})
  }, 3000)
}
function stopPolling() {
  if (polling.value) {
    clearInterval(polling.value)
    polling.value = null
  }
}

async function submitReview() {
  try {
    await tasksStore.review(taskId.value, {
      affected_products: reviewForm.products,
      affected_modules: reviewForm.modules,
    })
    notifySuccess('影响范围已提交')
  } catch {
    /* 错误已弹窗 */
  }
}

async function startGenerate() {
  generating.value = true
  try {
    await tasksStore.generate(taskId.value)
    notifySuccess('生成已触发，正在等待结果...')
    ensurePolling()
  } catch {
    /* 错误已弹窗 */
  } finally {
    generating.value = false
  }
}

async function retryTask() {
  retrying.value = true
  try {
    const updated = await tasksStore.retry(taskId.value)
    if (updated.status === 'analyzing') {
      notifySuccess('已重新触发分析，正在等待结果...')
      ensurePolling()
    } else {
      notifySuccess('任务已回到 ready_to_generate；可点击"开始生成"重新触发。')
    }
  } catch {
    /* 错误已弹窗 */
  } finally {
    retrying.value = false
  }
}

function findLastJobWithError() {
  for (let i = jobs.value.length - 1; i >= 0; i -= 1) {
    if (jobs.value[i].last_error) return jobs.value[i]
  }
  return null
}

function latestTraceError(value) {
  const rows = [
    ...(value.workflow_runs || []),
    ...(value.steps || []),
    ...(value.agent_runs || []),
    ...(value.model_calls || []),
    ...(value.retrieval_runs || []),
  ]
    .filter((row) => row.last_error)
    .sort((a, b) => new Date(b.updated_at || b.created_at || 0) - new Date(a.updated_at || a.created_at || 0))
  return compactTraceText(rows[0]?.last_error || '')
}

function compactTraceText(value) {
  if (!value) return ''
  return value.length > 160 ? `${value.slice(0, 157)}...` : value
}

function traceStatusLabel(status) {
  return {
    pending: 'pending',
    running: 'running',
    succeeded: 'succeeded',
    failed: 'failed',
    canceled: 'canceled',
  }[status] || status || '-'
}

function artifactLabel(artifact) {
  const payload = artifact.payload || {}
  const counts = []
  if (payload.section_count !== undefined) counts.push(`${payload.section_count} sections`)
  if (payload.case_count !== undefined) counts.push(`${payload.case_count} cases`)
  return counts.length ? counts.join(' · ') : artifact.artifact_type
}

function modelCallTokens(call) {
  const cost = call.metadata?.cost || {}
  return Number(cost.total_tokens || cost.estimated_total_tokens || Math.ceil(((call.prompt_chars || 0) + (call.response_chars || 0)) / 4) || 0)
}

function modelCallSignal(call) {
  const metadata = call.metadata || {}
  if (metadata.guardrail_event === 'budget_exceeded') return '预算耗尽'
  if (metadata.guardrail_event === 'circuit_open') return '熔断短路'
  if (metadata.provider_role === 'fallback') return 'fallback'
  if (String(call.last_error || '').toLowerCase().includes('rate limit')) return '限流'
  return ''
}

function findCaseProvenance(section, row, index) {
  return traceCaseProvenance.value.find((item) =>
    item.test_case_id === section.id &&
    (item.case_index === index || item.case_title === row.title),
  )
}

function feedbackTypeLabel(type) {
  return qualityFeedbackOptions.find((item) => item.value === type)?.label || type || '-'
}

function feedbackCountTotal(counts) {
  if (!counts) return 0
  return Object.values(counts).reduce((sum, value) => sum + Number(value || 0), 0)
}

function caseFeedbackCounts(section, row, index) {
  return findCaseProvenance(section, row, index)?.feedback_counts || {}
}

function latestCaseFeedback(section, row, index) {
  return latestFeedbackByCase.value.get(`${section.id}:${index ?? row.__case_index}`) || null
}

function caseReviewState(section, row, index) {
  const feedback = latestCaseFeedback(section, row, index)
  if (!feedback) return 'pending'
  if (feedback.feedback_type === 'useful') return 'passed'
  if (feedback.feedback_type === 'duplicate' && row.duplicate_hidden) return 'resolved'
  return 'issue'
}

function caseReviewLabel(section, row, index) {
  return { pending: '待审核', passed: '已通过', resolved: '已解决', issue: '有问题' }[caseReviewState(section, row, index)]
}

function caseReviewTagType(section, row, index) {
  return { pending: 'info', passed: 'success', resolved: 'success', issue: 'warning' }[caseReviewState(section, row, index)]
}

function sectionPendingReviewCount(section) {
  return (section.cases || []).filter((row, index) => caseReviewState(section, row, index) === 'pending').length
}

function sectionUnresolvedReviewCount(section) {
  return (section.cases || []).filter((row, index) => caseReviewState(section, row, index) === 'issue').length
}

function canSelectSection(section) {
  return !['submitted', 'approved'].includes(section.status) &&
    sectionPendingReviewCount(section) === 0 &&
    sectionUnresolvedReviewCount(section) === 0
}

function matchesCaseFilters(section, row, index) {
  if (caseFilters.hide_duplicates && row.duplicate_hidden) return false
  if (caseFilters.keyword) {
    const keyword = caseFilters.keyword.trim().toLowerCase()
    const searchable = [
      row.title,
      row.custom_preconds,
      ...(row.custom_steps_separated || []).flatMap((step) => [step.content, step.expected]),
    ].join('\n').toLowerCase()
    if (keyword && !searchable.includes(keyword)) return false
  }
  if (caseFilters.section && section.section !== caseFilters.section) return false
  if (caseFilters.priority_id && Number(row.priority_id) !== Number(caseFilters.priority_id)) return false
  if (caseFilters.product && !(row.affected_products || []).includes(caseFilters.product)) return false
  if (caseFilters.module && !(row.affected_modules || []).includes(caseFilters.module)) return false
  if (caseFilters.feedback_type) {
    const counts = caseFeedbackCounts(section, row, index)
    if (caseFilters.feedback_type === 'any') {
      if (!feedbackCountTotal(counts)) return false
    } else if (!counts[caseFilters.feedback_type]) {
      return false
    }
  }
  if (caseFilters.review_status && caseReviewState(section, row, index) !== caseFilters.review_status) return false
  if (caseFilters.provenance === 'with_sources' && !caseHasSources(section, row, index)) return false
  if (caseFilters.provenance === 'without_sources' && caseHasSources(section, row, index)) return false
  return true
}

function caseHasSources(section, row, index) {
  const provenance = findCaseProvenance(section, row, index)
  const source = provenance || section.source_context || {}
  return Boolean(
    (source.document_hits || []).length ||
      (source.knowledge_hits || []).length ||
      (source.document_queries || []).length ||
      (source.knowledge_queries || []).length,
  )
}

function reviewViewStorageKey() {
  return `caseagent.review-view.v1:${tenantStore.currentSlug || 'default'}:${taskId.value}`
}

function restoreReviewViewState() {
  let saved
  try {
    saved = JSON.parse(localStorage.getItem(reviewViewStorageKey()) || 'null')
  } catch {
    localStorage.removeItem(reviewViewStorageKey())
    return
  }
  if (!saved || saved.version !== 1) return

  const allowedFilters = [
    'keyword',
    'section',
    'priority_id',
    'product',
    'module',
    'feedback_type',
    'review_status',
    'provenance',
    'hide_duplicates',
  ]
  for (const key of allowedFilters) {
    if (Object.prototype.hasOwnProperty.call(saved.caseFilters || {}, key)) {
      caseFilters[key] = saved.caseFilters[key]
    }
  }
  overviewExpanded.value = saved.overviewExpanded !== false
  advancedFiltersVisible.value = Boolean(saved.advancedFiltersVisible)
  expandedSections.value = Array.isArray(saved.expandedSections) ? saved.expandedSections.map(Number).filter(Number.isInteger) : []
  expandedCaseRows.value = saved.expandedCaseRows && typeof saved.expandedCaseRows === 'object'
    ? saved.expandedCaseRows
    : {}
  focusedCaseKey.value = typeof saved.focusedCaseKey === 'string' ? saved.focusedCaseKey : ''
}

function persistReviewViewState() {
  if (!reviewViewReady.value) return
  localStorage.setItem(reviewViewStorageKey(), JSON.stringify({
    version: 1,
    caseFilters: { ...caseFilters },
    overviewExpanded: overviewExpanded.value,
    advancedFiltersVisible: advancedFiltersVisible.value,
    expandedSections: [...expandedSections.value],
    expandedCaseRows: { ...expandedCaseRows.value },
    focusedCaseKey: focusedCaseKey.value,
  }))
}

function pruneReviewViewState() {
  const validSections = new Map(cases.value.map((section) => [section.id, section]))
  expandedSections.value = expandedSections.value.filter((id) => validSections.has(Number(id)))

  const validExpandedRows = {}
  for (const [rawSectionID, keys] of Object.entries(expandedCaseRows.value || {})) {
    const sectionID = Number(rawSectionID)
    const section = validSections.get(sectionID)
    if (!section || !Array.isArray(keys)) continue
    const validKeys = new Set((section.cases || []).map((row, index) => `${sectionID}:${index}`))
    validExpandedRows[sectionID] = keys.filter((key) => validKeys.has(key))
  }
  expandedCaseRows.value = validExpandedRows

  if (focusedCaseKey.value) {
    const [rawSectionID, rawCaseIndex] = focusedCaseKey.value.split(':')
    const section = validSections.get(Number(rawSectionID))
    const caseIndex = Number(rawCaseIndex)
    if (!section || !Number.isInteger(caseIndex) || caseIndex < 0 || caseIndex >= (section.cases || []).length) {
      focusedCaseKey.value = ''
    }
  }
}

async function restoreFocusedCase() {
  if (restoredFocusApplied.value) return
  restoredFocusApplied.value = true
  if (!focusedCaseKey.value) return
  const [rawSectionID, rawCaseIndex] = focusedCaseKey.value.split(':')
  const section = cases.value.find((item) => item.id === Number(rawSectionID))
  const caseIndex = Number(rawCaseIndex)
  const row = section?.cases?.[caseIndex]
  if (!section || !row) return
  await focusCase(section, { ...row, __case_index: caseIndex }, 'auto')
}

function resetCaseFilters() {
  Object.assign(caseFilters, {
    keyword: '',
    section: '',
    priority_id: '',
    product: '',
    module: '',
    feedback_type: '',
    review_status: '',
    provenance: '',
    hide_duplicates: true,
  })
}

async function resetReviewView() {
  reviewViewReady.value = false
  localStorage.removeItem(reviewViewStorageKey())
  resetCaseFilters()
  overviewExpanded.value = true
  advancedFiltersVisible.value = false
  focusedCaseKey.value = ''
  clearCaseExpansion()
  await nextTick()
  reviewViewReady.value = true
  notifySuccess('审核视图已重置')
}

function sectionCaseRows(section) {
  return section.display_cases || []
}

function caseRowKey(section, row) {
  return `${section.id}:${row.__case_index}`
}

function caseAnchorId(section, row) {
  return `case-${caseRowKey(section, row).replace(':', '-')}`
}

function isSectionExpanded(section) {
  return expandedSections.value.includes(section.id)
}

function isCaseExpanded(section, row) {
  return (expandedCaseRows.value[section.id] || []).includes(caseRowKey(section, row))
}

function areSectionCasesExpanded(section) {
  const rows = sectionCaseRows(section)
  return rows.length > 0 && rows.every((row) => isCaseExpanded(section, row))
}

function ensureSectionExpanded(section) {
  if (isSectionExpanded(section)) return
  expandedSections.value = [...expandedSections.value, section.id]
}

function setSectionCasesExpanded(section, expanded) {
  if (expanded) ensureSectionExpanded(section)
  expandedCaseRows.value = {
    ...expandedCaseRows.value,
    [section.id]: expanded ? sectionCaseRows(section).map((row) => caseRowKey(section, row)) : [],
  }
}

function onCaseExpandChange(section, row, expanded) {
  const key = caseRowKey(section, row)
  const current = new Set(expandedCaseRows.value[section.id] || [])
  if (expanded) current.add(key)
  else current.delete(key)
  expandedCaseRows.value = { ...expandedCaseRows.value, [section.id]: [...current] }
}

function toggleCaseExpanded(section, row) {
  ensureSectionExpanded(section)
  onCaseExpandChange(section, row, !isCaseExpanded(section, row))
}

function expandAllCases() {
  expandedSections.value = filteredSections.value.map((section) => section.id)
  expandedCaseRows.value = Object.fromEntries(
    filteredSections.value.map((section) => [
      section.id,
      sectionCaseRows(section).map((row) => caseRowKey(section, row)),
    ]),
  )
}

function collapseAllCases() {
  clearCaseExpansion()
}

function toggleAllCaseDetails() {
  if (allCaseDetailsExpanded.value) collapseAllCases()
  else expandAllCases()
}

function toggleSectionCaseDetails(section) {
  setSectionCasesExpanded(section, !areSectionCasesExpanded(section))
}

function clearCaseExpansion() {
  expandedSections.value = []
  expandedCaseRows.value = {}
}

async function focusCase(section, row, behavior = 'smooth') {
  focusedCaseKey.value = caseRowKey(section, row)
  ensureSectionExpanded(section)
  onCaseExpandChange(section, row, true)
  await nextTick()
  document.getElementById(caseAnchorId(section, row))?.scrollIntoView({ behavior, block: 'center' })
}

function onCaseSelectionChange(section, selection) {
  selectedCaseRefs.value = {
    ...selectedCaseRefs.value,
    [section.id]: selection.map((row) => ({
      test_case_id: section.id,
      case_index: row.__case_index,
      title: row.title || '',
    })),
  }
}

function clearSelectedCases() {
  selectedCaseRefs.value = {}
}

function isSectionSelected(section) {
  return selectedSectionIds.value.includes(section.id)
}

function toggleSectionSelection(section, selected) {
  if (selected && !canSelectSection(section)) return
  const current = new Set(selectedSectionIds.value)
  if (selected) current.add(section.id)
  else current.delete(section.id)
  selectedSectionIds.value = [...current]
}

function openCaseProvenance(section, row, index) {
  const match = findCaseProvenance(section, row, index)
  provenanceCase.value = match || {
    test_case_id: section.id,
    section: section.section,
    case_index: index,
    case_title: row.title || '',
    source_context: section.source_context || {},
    document_queries: section.source_context?.document_queries || [],
    knowledge_queries: section.source_context?.knowledge_queries || [],
    document_hits: section.source_context?.document_hits || [],
    knowledge_hits: section.source_context?.knowledge_hits || [],
    agent_runs: section.source_context?.agent_runs || [],
    model_calls: section.source_context?.model_calls || [],
    feedback: [],
    feedback_counts: {},
  }
  provenanceTab.value = 'sources'
  provenanceVisible.value = true
}

function provenanceArray(key) {
  const value = provenanceCase.value?.[key] ?? provenanceCase.value?.source_context?.[key]
  return Array.isArray(value) ? value : []
}

function provenanceModelCalls() {
  const calls = provenanceArray('model_calls')
  return calls.length ? calls : traceModelCalls.value
}

function provenanceFeedbackRows() {
  return Array.isArray(provenanceCase.value?.feedback) ? provenanceCase.value.feedback : []
}

function provenanceAgentRuns() {
  const runs = provenanceArray('agent_runs')
  return runs.length ? runs : traceAgents.value
}

function provenanceTokenTotal() {
  return provenanceModelCalls().reduce((sum, call) => sum + modelCallTokens(call), 0)
}

function priorityLabel(id) {
  return { 1: 'Low', 2: 'Medium', 3: 'High', 4: 'Critical' }[id] || `P${id}`
}

function exportCases() {
  if (!cases.value.length) return
  const payload = {
    task_id: taskId.value,
    task_status: task.value?.status || '',
    exported_at: new Date().toISOString(),
    sections: cases.value,
  }
  downloadJSON(payload, `caseagent-task-${taskId.value}-test-cases.json`)
}

async function exportDiagnostics() {
  diagnosticsDownloading.value = true
  try {
    const payload = await getTaskDiagnostics(taskId.value)
    downloadJSON(payload, `caseagent-task-${taskId.value}-diagnostics.json`)
  } catch {
    /* api/client.js 已弹错 */
  } finally {
    diagnosticsDownloading.value = false
  }
}

function downloadJSON(payload, filename) {
  const blob = new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  document.body.appendChild(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
}

function openEditor(section) {
  editingCase.value = section
  editorBuffer.value = JSON.stringify(section.cases || [], null, 2)
  editorOriginal.value = editorBuffer.value
  editorVisible.value = true
}

async function saveEditor() {
  let parsed
  try {
    parsed = JSON.parse(editorBuffer.value)
    if (!Array.isArray(parsed)) throw new Error('cases 必须是数组')
  } catch (err) {
    ElMessageBox.alert(`JSON 解析失败：${err.message}`)
    return
  }
  if (!showCaseValidationResult(validateCaseRows(parsed, editingCase.value?.section || 'section'))) return
  try {
    await casesStore.update(taskId.value, editingCase.value.id, {
      section: editingCase.value.section,
      cases: parsed,
    })
    notifySuccess('用例已保存')
    editorOriginal.value = JSON.stringify(parsed, null, 2)
    editorVisible.value = false
  } catch {
    /* 错误已弹窗 */
  }
}

async function submitSection(section) {
  const pendingCount = sectionPendingReviewCount(section)
  const unresolvedCount = sectionUnresolvedReviewCount(section)
  if (pendingCount > 0) {
    await ElMessageBox.alert(`类别「${section.section}」还有 ${pendingCount} 条用例待审核，全部审核后才能提交。`)
    return
  }
  if (unresolvedCount > 0) {
    await ElMessageBox.alert(`类别「${section.section}」还有 ${unresolvedCount} 条问题用例未解决。请编辑后重新标记通过，或完成重复项处置。`)
    return
  }
  if (!showCaseValidationResult(validateSection(section))) return
  try {
    await ElMessageBox.confirm(
      `提交类别「${section.section}」共 ${section.cases?.length || 0} 条用例？`,
      '确认',
      { type: 'info' },
    )
  } catch {
    return
  }
  try {
    await casesStore.submit(taskId.value, section.id)
    toggleSectionSelection(section, false)
    notifySuccess('已提交')
  } catch {
    /* 错误已弹窗 */
  }
}

async function submitSelectedSections() {
  const ids = [...selectedSectionIds.value]
  if (!ids.length) {
    ElMessageBox.alert('请先选择要提交的类别')
    return
  }
  const sections = cases.value.filter((section) => ids.includes(section.id))
  const pendingCount = sections.reduce((sum, section) => sum + sectionPendingReviewCount(section), 0)
  const unresolvedCount = sections.reduce((sum, section) => sum + sectionUnresolvedReviewCount(section), 0)
  if (pendingCount > 0) {
    await ElMessageBox.alert(`已选类别还有 ${pendingCount} 条用例待审核，全部审核后才能提交。`)
    return
  }
  if (unresolvedCount > 0) {
    await ElMessageBox.alert(`已选类别还有 ${unresolvedCount} 条问题用例未解决，处理完成后才能提交。`)
    return
  }
  if (!showCaseValidationResult(sections.flatMap((section) => validateSection(section)))) return
  try {
    const caseCount = sections.reduce((sum, section) => sum + (section.cases?.length || 0), 0)
    await ElMessageBox.confirm(`提交已选的 ${ids.length} 个类别，共 ${caseCount} 条用例？`, '确认提交', { type: 'info' })
  } catch {
    return
  }
  try {
    await casesStore.batchSubmit(taskId.value, ids)
    selectedSectionIds.value = []
    notifySuccess('已提交选中类别')
  } catch {
    /* 错误已弹窗 */
  }
}

async function applyBatchPatch() {
  if (!selectedCaseCount.value) {
    ElMessageBox.alert('请先选择用例')
    return
  }
  const patch = {}
  if (batchPatch.priority_id) patch.priority_id = Number(batchPatch.priority_id)
  if (batchPatch.affected_products.length) patch.affected_products = [...batchPatch.affected_products]
  if (batchPatch.affected_modules.length) patch.affected_modules = [...batchPatch.affected_modules]
  if (!Object.keys(patch).length) {
    ElMessageBox.alert('请选择要批量修改的字段')
    return
  }
  try {
    await casesStore.batchUpdate(taskId.value, {
      cases: selectedCases.value.map(({ test_case_id, case_index }) => ({ test_case_id, case_index })),
      patch,
    })
    notifySuccess('批量修改已保存')
  } catch {
    /* 错误已弹窗 */
  }
}

async function hideSelectedDuplicates() {
  if (!selectedCaseCount.value) {
    ElMessageBox.alert('请先选择用例')
    return
  }
  try {
    await casesStore.batchFeedback(taskId.value, {
      cases: selectedCases.value.map(({ test_case_id, case_index }) => ({ test_case_id, case_index })),
      feedback_type: 'duplicate',
      note: '批量标记为重复用例',
    })
    await casesStore.batchUpdate(taskId.value, {
      cases: selectedCases.value.map(({ test_case_id, case_index }) => ({ test_case_id, case_index })),
      patch: { duplicate_hidden: true },
    })
    clearSelectedCases()
    notifySuccess('已标记重复并隐藏')
    loadTrace().catch(() => {})
  } catch {
    /* 错误已弹窗 */
  }
}

async function passSelectedCases() {
  if (!selectedCaseCount.value) {
    ElMessageBox.alert('请先选择用例')
    return
  }
  try {
    await ElMessageBox.confirm(`将选中的 ${selectedCaseCount.value} 条用例标记为通过？`, '批量通过', { type: 'info' })
  } catch {
    return
  }
  try {
    await casesStore.batchFeedback(taskId.value, {
      cases: selectedCases.value.map(({ test_case_id, case_index }) => ({ test_case_id, case_index })),
      feedback_type: 'useful',
      note: '批量审核通过',
    })
    clearSelectedCases()
    await loadTrace()
    notifySuccess('选中用例已批量通过')
  } catch {
    /* api/client.js 已弹错 */
  }
}

function openCaseEditor(section, row) {
  editingCaseSection.value = section
  editingCaseIndex.value = row.__case_index
  Object.assign(caseForm, {
    title: row.title || '',
    priority_id: Number(row.priority_id || 2),
    custom_preconds: row.custom_preconds || '',
    affected_products: [...(row.affected_products || [])],
    affected_modules: [...(row.affected_modules || [])],
    custom_steps_separated: (row.custom_steps_separated || []).map((step) => ({
      content: step.content || '',
      expected: step.expected || '',
    })),
  })
  if (!caseForm.custom_steps_separated.length) {
    caseForm.custom_steps_separated.push({ content: '', expected: '' })
  }
  caseEditorOriginal.value = serializeCaseForm()
  caseEditorVisible.value = true
}

function addCaseStep() {
  caseForm.custom_steps_separated.push({ content: '', expected: '' })
}

function removeCaseStep(index) {
  caseForm.custom_steps_separated.splice(index, 1)
  if (!caseForm.custom_steps_separated.length) {
    addCaseStep()
  }
}

function serializeCaseForm() {
  return JSON.stringify({
    title: caseForm.title,
    priority_id: Number(caseForm.priority_id || 0),
    custom_preconds: caseForm.custom_preconds,
    affected_products: [...caseForm.affected_products],
    affected_modules: [...caseForm.affected_modules],
    custom_steps_separated: caseForm.custom_steps_separated.map((step) => ({
      content: step.content,
      expected: step.expected,
    })),
  })
}

function validateSection(section) {
  return validateCaseRows(section.cases || [], section.section || `section #${section.id}`)
}

function validateCaseRows(rows, sectionLabel) {
  if (!Array.isArray(rows) || !rows.length) return [`${sectionLabel}: 至少需要 1 条用例`]
  return rows.flatMap((row, index) => validateCaseRow(row, `${sectionLabel} / case #${index + 1}`))
}

function validateCaseRow(row, label) {
  const errors = []
  if (!String(row?.title || '').trim()) {
    errors.push(`${label}: title 不能为空`)
  }
  const priority = Number(row?.priority_id)
  if (!Number.isInteger(priority) || priority < 1 || priority > 4) {
    errors.push(`${label}: priority_id 必须是 1-4`)
  }
  const steps = Array.isArray(row?.custom_steps_separated) ? row.custom_steps_separated : []
  const completeSteps = steps.filter((step) => String(step?.content || '').trim() && String(step?.expected || '').trim())
  if (!completeSteps.length) {
    errors.push(`${label}: 至少需要 1 个完整步骤（操作和预期都必填）`)
  }
  const incompleteStepIndex = steps.findIndex((step) => {
    const hasContent = String(step?.content || '').trim()
    const hasExpected = String(step?.expected || '').trim()
    return (hasContent || hasExpected) && !(hasContent && hasExpected)
  })
  if (incompleteStepIndex >= 0) {
    errors.push(`${label}: 第 ${incompleteStepIndex + 1} 步操作和预期必须同时填写`)
  }
  if (!trimmedArray(row?.affected_products).length) {
    errors.push(`${label}: affected_products 不能为空`)
  }
  if (!trimmedArray(row?.affected_modules).length) {
    errors.push(`${label}: affected_modules 不能为空`)
  }
  return errors
}

function trimmedArray(value) {
  return Array.isArray(value) ? value.map((item) => String(item || '').trim()).filter(Boolean) : []
}

function showCaseValidationResult(errors) {
  if (!errors.length) return true
  const visibleErrors = errors.slice(0, 12)
  const suffix = errors.length > visibleErrors.length ? `\n... 还有 ${errors.length - visibleErrors.length} 条` : ''
  ElMessageBox.alert(`${visibleErrors.join('\n')}${suffix}`, '提交前校验失败', { type: 'warning' })
  return false
}

async function saveCaseEditor() {
  if (!editingCaseSection.value || editingCaseIndex.value < 0) return
  const nextCases = (editingCaseSection.value.cases || []).map((row) => ({ ...row }))
  const nextCase = {
    ...nextCases[editingCaseIndex.value],
    title: caseForm.title.trim(),
    priority_id: Number(caseForm.priority_id || 2),
    custom_preconds: caseForm.custom_preconds.trim(),
    affected_products: [...caseForm.affected_products],
    affected_modules: [...caseForm.affected_modules],
    custom_steps_separated: caseForm.custom_steps_separated
      .map((step) => ({ content: step.content.trim(), expected: step.expected.trim() }))
      .filter((step) => step.content || step.expected),
  }
  if (!showCaseValidationResult(validateCaseRow(nextCase, '当前用例'))) return
  nextCases[editingCaseIndex.value] = nextCase
  try {
    await casesStore.update(taskId.value, editingCaseSection.value.id, {
      section: editingCaseSection.value.section,
      cases: nextCases,
    })
    notifySuccess('用例已保存')
    caseEditorOriginal.value = serializeCaseForm()
    caseEditorVisible.value = false
  } catch {
    /* 错误已弹窗 */
  }
}

function openQualityFeedback(section, row, index, initialType = 'requirement_mismatch') {
  Object.assign(qualityFeedbackForm, {
    test_case_id: section.id,
    case_index: index,
    case_title: row.title || section.section || '',
    feedback_type: initialType,
    note: '',
  })
  qualityFeedbackVisible.value = true
}

async function markCasePassed(section, row, index) {
  try {
    await casesStore.feedback(taskId.value, section.id, {
      case_index: index,
      feedback_type: 'useful',
      note: '',
    })
    notifySuccess('已标记为通过')
    await loadTrace()
  } catch {
    /* 错误已弹窗 */
  }
}

async function submitQualityFeedback() {
  try {
    await casesStore.feedback(taskId.value, qualityFeedbackForm.test_case_id, {
      case_index: qualityFeedbackForm.case_index,
      feedback_type: qualityFeedbackForm.feedback_type,
      note: qualityFeedbackForm.note.trim(),
    })
    qualityFeedbackVisible.value = false
    notifySuccess('用例质量反馈已提交')
    loadTrace().catch(() => {})
  } catch {
    /* 错误已弹窗 */
  }
}

function openKnowledgeFeedback(section, row) {
  const moduleName = (row.affected_modules || [])[0] || ''
  const productName = (row.affected_products || [])[0] || ''
  Object.assign(feedbackForm, {
    candidate_type: moduleName ? 'module' : 'product',
    candidate_name: moduleName || productName || '',
    source_case_id: section.id,
    source_case_index: row.__case_index,
    source_task_id: taskId.value,
    source_case_title: row.title || section.section || '',
    note: '',
  })
  feedbackVisible.value = true
}

async function submitKnowledgeFeedback() {
  if (!feedbackForm.candidate_name.trim()) {
    ElMessageBox.alert('请输入候选名称')
    return
  }
  try {
    await suggestionStore.createManual({
      candidate_type: feedbackForm.candidate_type,
      candidate_name: feedbackForm.candidate_name.trim(),
      source_case_id: feedbackForm.source_case_id,
      source_task_id: feedbackForm.source_task_id,
      source_case_title: feedbackForm.source_case_title,
      note: feedbackForm.note.trim(),
    })
    await casesStore.feedback(taskId.value, feedbackForm.source_case_id, {
      case_index: feedbackForm.source_case_index,
      feedback_type: 'knowledge_missing',
      note: feedbackForm.note.trim(),
    })
    feedbackVisible.value = false
    notifySuccess('知识缺失反馈已提交')
    loadTrace().catch(() => {})
  } catch {
    /* 错误已弹窗 */
  }
}
</script>

<template>
  <section v-if="task" class="task-detail">
    <header class="page-header">
      <el-breadcrumb separator="/">
        <el-breadcrumb-item :to="{ name: 'projects' }">项目</el-breadcrumb-item>
        <el-breadcrumb-item
          :to="{ name: 'project-detail', params: { id: task.project_id } }"
        >项目 #{{ task.project_id }}</el-breadcrumb-item>
        <el-breadcrumb-item>任务 #{{ task.id }}</el-breadcrumb-item>
      </el-breadcrumb>
      <div class="task-meta">
        <StatusTag :status="task.status" />
        <span class="muted">文档数：{{ (task.document_ids || []).length }}</span>
        <span class="muted">更新：{{ formatDate(task.updated_at) }}</span>
        <el-button
          v-if="task.status === 'failed'"
          type="warning"
          size="small"
          :loading="retrying"
          @click="retryTask"
        >重试</el-button>
        <el-button
          v-if="task.status === 'failed'"
          size="small"
          @click="loadTask"
        >刷新状态</el-button>
        <el-tag v-if="isPolling" type="warning" size="small">轮询中（3s）</el-tag>
      </div>
      <p v-if="task.status === 'failed'" class="muted danger">
        任务失败。常见原因：模型 API 失败、子 Agent 全部失败且 DeepAgent fallback 也失败。
        点击“重试”会根据当前状态自动回到“分析中”或“待生成”；若回到“待生成”，
        请确认根因（后端日志关键字 <code>agent</code> / <code>document</code>）已修复后再"开始生成"。
      </p>
    </header>

    <div class="task-view-switch">
      <el-radio-group v-model="activeTaskView">
        <el-radio-button value="review">用例审核</el-radio-button>
        <el-radio-button value="diagnostics">技术诊断</el-radio-button>
      </el-radio-group>
    </div>

    <el-card v-if="activeTaskView === 'review' && canReview" shadow="never" class="card">
      <template #header><span>影响范围审核</span></template>
      <el-form label-width="100px">
        <el-form-item label="受影响产品">
          <el-select
            v-model="reviewForm.products"
            multiple
            filterable
            allow-create
            style="width: 100%"
            placeholder="留空表示不限定"
          >
            <el-option v-for="p in productOptions" :key="p" :value="p" :label="p" />
          </el-select>
        </el-form-item>
        <el-form-item label="受影响模块">
          <el-select
            v-model="reviewForm.modules"
            multiple
            filterable
            allow-create
            style="width: 100%"
            placeholder="留空表示不限定"
          >
            <el-option v-for="m in moduleOptions" :key="m" :value="m" :label="m" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button
            type="primary"
            @click="submitReview"
          >提交审核</el-button>
          <el-button
            type="success"
            :disabled="!canGenerate"
            :loading="generating"
            @click="startGenerate"
          >开始生成</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card
      v-else-if="activeTaskView === 'review' && ['completed', 'failed'].includes(task.status)"
      shadow="never"
      class="card"
    >
      <template #header><span>影响范围</span></template>
      <div class="readonly-scope">
        <div>
          <span>受影响产品</span>
          <div>
            <el-tag v-for="item in task.affected_products || []" :key="item" size="small" type="info">
              {{ item }}
            </el-tag>
            <span v-if="!(task.affected_products || []).length" class="muted">未限定</span>
          </div>
        </div>
        <div>
          <span>受影响模块</span>
          <div>
            <el-tag v-for="item in task.affected_modules || []" :key="item" size="small">
              {{ item }}
            </el-tag>
            <span v-if="!(task.affected_modules || []).length" class="muted">未限定</span>
          </div>
        </div>
      </div>
    </el-card>

    <el-card v-show="activeTaskView === 'review'" shadow="never" class="card case-output-card">
      <template #header>
        <div class="card-header">
          <span>测试用例输出</span>
          <div class="header-actions">
            <el-tag type="info" size="small">{{ caseSectionCount }} 个类别</el-tag>
            <el-tag type="success" size="small">{{ totalCaseCount }} 条用例</el-tag>
            <el-tag type="success" effect="plain" size="small">
              已审核 {{ reviewedCaseCount }}/{{ totalCaseCount }}
            </el-tag>
            <el-tag v-if="passedCaseCount" type="success" size="small">
              通过 {{ passedCaseCount }}
            </el-tag>
            <el-tag v-if="resolvedCaseCount" type="success" effect="plain" size="small">
              已解决 {{ resolvedCaseCount }}
            </el-tag>
            <el-tag v-if="issueCaseCount" type="warning" size="small">
              未解决 {{ issueCaseCount }}
            </el-tag>
            <el-tag v-if="filteredCaseCount !== totalCaseCount" type="warning" size="small">
              当前显示 {{ filteredCaseCount }} 条
            </el-tag>
            <el-button
              :icon="Refresh"
              @click="refreshCases"
              :loading="casesLoading"
              :disabled="!['completed', 'failed'].includes(task.status)"
            >刷新</el-button>
            <el-button
              type="primary"
              :icon="Download"
              :disabled="!cases.length"
              @click="exportCases"
            >导出 JSON</el-button>
          </div>
        </div>
      </template>
      <el-empty
        v-if="!totalCaseCount && !casesLoading"
        :description="caseOutputEmptyDescription"
      />
      <template v-else>
        <div class="case-overview">
          <div class="overview-header">
            <div>
              <h3>用例概览</h3>
              <p class="muted small">{{ filteredCaseCount }} 条标题，按类别汇总</p>
            </div>
            <div class="overview-actions">
              <el-button
                size="small"
                :icon="overviewExpanded ? ArrowUpBold : ArrowDownBold"
                @click="overviewExpanded = !overviewExpanded"
              >
                {{ overviewExpanded ? '收起概览' : '展开概览' }}
              </el-button>
            </div>
          </div>
          <el-collapse-transition>
            <div v-show="overviewExpanded">
              <el-scrollbar max-height="380px" class="overview-scroll">
                <div class="overview-sections">
                  <section
                    v-for="section in filteredSections"
                    :key="`overview-${section.id}`"
                    class="overview-section"
                  >
                    <div class="overview-section-header">
                      <strong>{{ section.section }}</strong>
                      <el-tag size="small" type="info">{{ section.display_cases.length }} 条</el-tag>
                    </div>
                    <ol class="overview-title-list">
                      <li v-for="row in section.display_cases" :key="caseRowKey(section, row)">
                        <el-button link type="primary" @click="focusCase(section, row)">
                          {{ row.title || `用例 #${row.__case_index + 1}` }}
                        </el-button>
                      </li>
                    </ol>
                  </section>
                </div>
              </el-scrollbar>
            </div>
          </el-collapse-transition>
        </div>

        <div class="case-review-tools">
          <div class="case-detail-toolbar">
            <span>用例详情</span>
            <div>
              <span class="section-selection-label">选择已全部审核的类别提交</span>
              <el-tag type="info" size="small">已选 {{ selectedSectionCount }} 个类别</el-tag>
              <el-button
                size="small"
                type="primary"
                :icon="Check"
                :disabled="!selectedSectionCount"
                :loading="batchSaving"
                @click="submitSelectedSections"
              >提交已选类别</el-button>
              <el-button
                size="small"
                :icon="allCaseDetailsExpanded ? ArrowUpBold : ArrowDownBold"
                @click="toggleAllCaseDetails"
              >
                {{ allCaseDetailsExpanded ? '收起全部用例详情' : '展开全部用例详情' }}
              </el-button>
            </div>
          </div>
          <div class="filter-bar case-filter-bar">
            <el-input
              v-model="caseFilters.keyword"
              clearable
              placeholder="搜索标题、前置条件或步骤"
              class="filter-control keyword-filter"
              :prefix-icon="Search"
            />
            <el-select v-model="caseFilters.section" clearable placeholder="类别" class="filter-control">
              <el-option v-for="item in sectionOptions" :key="item" :label="item" :value="item" />
            </el-select>
            <el-select v-model="caseFilters.review_status" clearable placeholder="审核状态" class="filter-control">
              <el-option label="待审核" value="pending" />
              <el-option label="已通过" value="passed" />
              <el-option label="已解决" value="resolved" />
              <el-option label="有问题" value="issue" />
            </el-select>
            <el-button
              :icon="Filter"
              :type="advancedFiltersVisible || hasAdvancedCaseFilters ? 'primary' : ''"
              plain
              @click="advancedFiltersVisible = !advancedFiltersVisible"
            >{{ advancedFiltersVisible ? '收起筛选' : '更多筛选' }}</el-button>
            <el-button @click="resetCaseFilters">重置筛选</el-button>
            <el-button @click="resetReviewView">重置视图</el-button>
          </div>

          <div v-show="advancedFiltersVisible" class="filter-bar case-filter-bar advanced-filter-bar">
            <el-select v-model="caseFilters.priority_id" clearable placeholder="优先级" class="filter-control compact">
              <el-option v-for="item in priorityOptions" :key="item.value" :label="item.label" :value="item.value" />
            </el-select>
            <el-select v-model="caseFilters.product" clearable filterable placeholder="产品" class="filter-control">
              <el-option v-for="item in productOptions" :key="item" :label="item" :value="item" />
            </el-select>
            <el-select v-model="caseFilters.module" clearable filterable placeholder="模块" class="filter-control">
              <el-option v-for="item in moduleOptions" :key="item" :label="item" :value="item" />
            </el-select>
            <el-select v-model="caseFilters.feedback_type" clearable placeholder="反馈" class="filter-control">
              <el-option label="有反馈" value="any" />
              <el-option
                v-for="item in qualityFeedbackOptions"
                :key="item.value"
                :label="item.label"
                :value="item.value"
              />
            </el-select>
            <el-select v-model="caseFilters.provenance" clearable placeholder="依据" class="filter-control compact">
              <el-option label="有依据" value="with_sources" />
              <el-option label="无依据" value="without_sources" />
            </el-select>
            <el-switch v-model="caseFilters.hide_duplicates" active-text="隐藏重复" />
          </div>

          <div v-if="selectedCaseCount" class="batch-bar">
            <el-tag type="info" size="small">已选 {{ selectedCaseCount }} 条</el-tag>
            <el-select v-model="batchPatch.priority_id" clearable placeholder="优先级" class="batch-control compact">
              <el-option v-for="item in priorityOptions" :key="item.value" :label="item.label" :value="item.value" />
            </el-select>
            <el-select
              v-model="batchPatch.affected_products"
              multiple
              filterable
              allow-create
              collapse-tags
              placeholder="产品"
              class="batch-control"
            >
              <el-option v-for="item in productOptions" :key="item" :label="item" :value="item" />
            </el-select>
            <el-select
              v-model="batchPatch.affected_modules"
              multiple
              filterable
              allow-create
              collapse-tags
              placeholder="模块"
              class="batch-control"
            >
              <el-option v-for="item in moduleOptions" :key="item" :label="item" :value="item" />
            </el-select>
            <el-button type="success" :icon="Check" :loading="feedbackSaving" @click="passSelectedCases">批量通过</el-button>
            <el-button :icon="EditPen" :loading="batchSaving" @click="applyBatchPatch">批量修改</el-button>
            <el-button :icon="Warning" :loading="batchSaving" @click="hideSelectedDuplicates">标记重复</el-button>
          </div>
        </div>
        <el-empty
          v-if="!filteredCaseCount && !casesLoading"
          description="没有符合筛选条件的用例"
        />
        <el-collapse v-else v-model="expandedSections">
          <el-collapse-item
            v-for="section in filteredSections"
            :key="section.id"
            :name="section.id"
          >
          <template #title>
            <div class="section-title">
              <span @click.stop>
                <el-checkbox
                  :model-value="isSectionSelected(section)"
                  :disabled="!canSelectSection(section)"
                  :title="sectionPendingReviewCount(section)
                    ? `还有 ${sectionPendingReviewCount(section)} 条待审核`
                    : sectionUnresolvedReviewCount(section)
                      ? `还有 ${sectionUnresolvedReviewCount(section)} 条问题未解决`
                      : '选择类别提交'"
                  @change="(selected) => toggleSectionSelection(section, selected)"
                />
              </span>
              <span>{{ section.section }}</span>
              <el-tag size="small" type="info">
                {{ section.display_cases?.length || 0 }}/{{ section.cases?.length || 0 }} 条
              </el-tag>
              <el-tag v-if="sectionPendingReviewCount(section)" size="small" type="warning">
                待审核 {{ sectionPendingReviewCount(section) }} 条
              </el-tag>
              <el-tag v-if="sectionUnresolvedReviewCount(section)" size="small" type="danger">
                未解决 {{ sectionUnresolvedReviewCount(section) }} 条
              </el-tag>
              <el-tag v-if="!sectionPendingReviewCount(section) && !sectionUnresolvedReviewCount(section)" size="small" type="success">
                已可提交
              </el-tag>
              <StatusTag :status="section.status" />
            </div>
          </template>

          <div class="section-actions">
            <el-button
              size="small"
              :icon="areSectionCasesExpanded(section) ? ArrowUpBold : ArrowDownBold"
              @click="toggleSectionCaseDetails(section)"
            >{{ areSectionCasesExpanded(section) ? '收起本类详情' : '展开本类详情' }}</el-button>
            <el-button size="small" :icon="EditPen" @click="openEditor(section)">编辑 JSON</el-button>
            <el-button
              size="small"
              type="primary"
              :icon="Check"
              :disabled="!canSelectSection(section)"
              @click="submitSection(section)"
            >提交类别</el-button>
          </div>

          <el-table
            :data="section.display_cases || []"
            stripe
            size="small"
            :row-key="(row) => caseRowKey(section, row)"
            :expand-row-keys="expandedCaseRows[section.id] || []"
            @expand-change="(row, expanded) => onCaseExpandChange(section, row, expanded)"
            @selection-change="(selection) => onCaseSelectionChange(section, selection)"
          >
            <el-table-column type="selection" width="44" fixed="left" />
            <el-table-column type="expand" width="48" fixed="left">
              <template #default="{ row }">
                <div class="case-expand">
                  <div class="case-field">
                    <div class="field-label">前置条件</div>
                    <div class="field-body">{{ row.custom_preconds || '-' }}</div>
                  </div>
                  <div class="case-field">
                    <div class="field-label">步骤</div>
                    <el-table
                      :data="row.custom_steps_separated || []"
                      size="small"
                      border
                      class="steps-table"
                    >
                      <el-table-column label="#" width="56">
                        <template #default="{ $index }">{{ $index + 1 }}</template>
                      </el-table-column>
                      <el-table-column prop="content" label="操作" min-width="260" />
                      <el-table-column prop="expected" label="预期" min-width="260" />
                    </el-table>
                  </div>
                </div>
              </template>
            </el-table-column>
            <el-table-column prop="title" label="标题" min-width="320" fixed="left">
              <template #default="{ row }">
                <el-button
                  :id="caseAnchorId(section, row)"
                  link
                  type="primary"
                  class="case-title-button"
                  @click="toggleCaseExpanded(section, row)"
                >{{ row.title || `用例 #${row.__case_index + 1}` }}</el-button>
              </template>
            </el-table-column>
            <el-table-column label="优先级" width="100">
              <template #default="{ row }">{{ priorityLabel(row.priority_id) }}</template>
            </el-table-column>
            <el-table-column label="影响产品" min-width="140">
              <template #default="{ row }">
                <el-tag
                  v-for="p in row.affected_products || []"
                  :key="p"
                  size="small"
                  type="info"
                  class="chip"
                >{{ p }}</el-tag>
                <span v-if="!(row.affected_products || []).length" class="muted">-</span>
              </template>
            </el-table-column>
            <el-table-column label="影响模块" min-width="140">
              <template #default="{ row }">
                <el-tag
                  v-for="m in row.affected_modules || []"
                  :key="m"
                  size="small"
                  class="chip"
                >{{ m }}</el-tag>
                <span v-if="!(row.affected_modules || []).length" class="muted">-</span>
              </template>
            </el-table-column>
            <el-table-column prop="custom_preconds" label="前置条件" min-width="220" show-overflow-tooltip />
            <el-table-column label="步骤数" width="90">
              <template #default="{ row }">{{ (row.custom_steps_separated || []).length }}</template>
            </el-table-column>
            <el-table-column label="编辑" width="100" align="center">
              <template #default="{ row }">
                <el-button
                  size="small"
                  :icon="EditPen"
                  @click="openCaseEditor(section, row)"
                >编辑</el-button>
              </template>
            </el-table-column>
            <el-table-column label="依据" width="110" align="center">
              <template #default="{ row }">
                <el-button
                  size="small"
                  :icon="View"
                  @click="openCaseProvenance(section, row, row.__case_index)"
                >查看依据</el-button>
              </template>
            </el-table-column>
            <el-table-column label="审核" width="270" align="center" fixed="right">
              <template #default="{ row }">
                <div class="case-review-actions">
                  <el-tag size="small" :type="caseReviewTagType(section, row, row.__case_index)">
                    {{ caseReviewLabel(section, row, row.__case_index) }}
                  </el-tag>
                  <el-button
                    size="small"
                    type="success"
                    link
                    :loading="feedbackSaving"
                    @click="markCasePassed(section, row, row.__case_index)"
                  >通过</el-button>
                  <el-button
                    size="small"
                    link
                    :icon="ChatDotRound"
                    @click="openQualityFeedback(section, row, row.__case_index)"
                  >有问题</el-button>
                  <el-button
                    size="small"
                    link
                    :icon="Warning"
                    @click="openKnowledgeFeedback(section, row)"
                  >知识缺失</el-button>
                </div>
              </template>
            </el-table-column>
          </el-table>

          </el-collapse-item>
        </el-collapse>
      </template>
    </el-card>

    <el-card
      v-show="activeTaskView === 'diagnostics'"
      shadow="never"
      class="card diagnostics-card"
      v-loading="traceLoading"
    >
      <template #header>
        <div class="card-header">
          <span>诊断信息</span>
          <div class="header-actions">
            <el-button size="small" :loading="jobsLoading" @click="loadJobs">刷新任务</el-button>
            <el-button size="small" :loading="traceLoading" @click="loadTrace">刷新 Trace</el-button>
            <el-button
              size="small"
              type="primary"
              :icon="Download"
              :loading="diagnosticsDownloading"
              @click="exportDiagnostics"
            >导出诊断包</el-button>
          </div>
        </div>
      </template>

      <div class="diagnostic-summary">
        <el-tag size="small" type="info">{{ jobTimeline.length }} timeline</el-tag>
        <el-tag size="small" type="info">{{ traceSummary.workflows }} runs</el-tag>
        <el-tag size="small" type="info">{{ traceSummary.steps }} steps</el-tag>
        <el-tag size="small" type="info">{{ traceSummary.agents }} agents</el-tag>
        <el-tag size="small" type="info">{{ traceSummary.modelCalls }} model calls</el-tag>
        <el-tag size="small" type="success">{{ traceCostSummary.tokens }} tokens</el-tag>
        <el-tag v-if="traceSummary.feedback" size="small" type="warning">
          {{ traceSummary.feedback }} feedback
        </el-tag>
        <el-tag v-if="traceCostSummary.fallbacks" size="small" type="warning">
          {{ traceCostSummary.fallbacks }} fallback
        </el-tag>
        <el-tag v-if="traceCostSummary.guardrails" size="small" type="danger">
          {{ traceCostSummary.guardrails }} guardrail
        </el-tag>
        <el-tag v-if="traceCostSummary.rateLimits" size="small" type="warning">
          {{ traceCostSummary.rateLimits }} rate limit
        </el-tag>
        <el-tag size="small" type="info">{{ traceSummary.retrievals }} retrievals</el-tag>
        <el-tag size="small" type="info">{{ traceSummary.artifacts }} artifacts</el-tag>
      </div>
      <p v-if="traceSummary.lastError || lastJobError" class="muted danger trace-error">
        {{ traceSummary.lastError || lastJobError }}
      </p>

      <el-collapse class="diagnostic-collapse">
        <el-collapse-item name="jobs">
          <template #title>
            <span class="diagnostic-title">后台任务 timeline</span>
          </template>
          <el-timeline class="job-timeline">
            <el-timeline-item
              v-for="item in jobTimeline"
              :key="item.key"
              :type="jobStatusType(item.status)"
              :timestamp="formatDate(item.time)"
              placement="top"
            >
              <div class="job-row">
                <div class="job-row-main">
                  <span class="job-label">{{ item.label }}</span>
                  <el-tag size="small" :type="jobStatusType(item.status)">
                    {{ jobStatusLabel(item.status) }} ({{ item.status }})
                  </el-tag>
                  <span v-if="item.retry" class="muted small">retry {{ item.retry }}</span>
                  <span v-if="item.nextRun" class="muted small">next {{ formatDate(item.nextRun) }}</span>
                </div>
                <p v-if="item.error" class="muted danger job-error">{{ item.error }}</p>
              </div>
            </el-timeline-item>
          </el-timeline>
        </el-collapse-item>

        <el-collapse-item name="trace">
          <template #title>
            <span class="diagnostic-title">Workflow Trace / Model / Retrieval</span>
          </template>
          <el-empty
            v-if="!hasTraceData && !traceLoading"
            description="暂无 workflow trace"
          />
          <div v-if="hasTraceData" class="trace-layout">
            <div class="trace-column">
              <h3>Runs</h3>
              <div v-for="run in traceRuns" :key="run.id" class="trace-row">
                <div class="trace-row-main">
                  <span class="trace-title">{{ jobTypeLabel(run.workflow_type) }} ({{ run.workflow_type }})</span>
                  <el-tag size="small" :type="jobStatusType(run.status)">
                    {{ traceStatusLabel(run.status) }} ({{ run.status }})
                  </el-tag>
                </div>
                <div class="muted small">
                  #{{ run.id }} · {{ formatDate(run.started_at || run.created_at) }}
                </div>
                <p v-if="run.last_error" class="muted danger trace-error">
                  {{ compactTraceText(run.last_error) }}
                </p>
              </div>
            </div>
            <div class="trace-column">
              <h3>Agents</h3>
              <div v-for="agent in traceAgents" :key="agent.id" class="trace-row">
                <div class="trace-row-main">
                  <span class="trace-title">{{ agent.agent_name }}</span>
                  <el-tag size="small" :type="jobStatusType(agent.status)">
                    {{ traceStatusLabel(agent.status) }} ({{ agent.status }})
                  </el-tag>
                </div>
                <div class="muted small">{{ agent.stage }} · {{ formatDate(agent.finished_at || agent.created_at) }}</div>
                <p v-if="agent.output_summary" class="trace-snippet">
                  {{ compactTraceText(agent.output_summary) }}
                </p>
                <p v-if="agent.last_error" class="muted danger trace-error">
                  {{ compactTraceText(agent.last_error) }}
                </p>
              </div>
              <el-empty v-if="!traceAgents.length && hasTraceData" description="暂无 agent runs" />
            </div>
            <div class="trace-column">
              <h3>Models</h3>
              <div v-for="call in traceModelCalls" :key="call.id" class="trace-row">
                <div class="trace-row-main">
                  <span class="trace-title">{{ call.provider || '-' }} / {{ call.model || '-' }}</span>
                  <el-tag size="small" :type="jobStatusType(call.status)">
                    {{ traceStatusLabel(call.status) }} ({{ call.status }})
                  </el-tag>
                </div>
                <div class="muted small">
                  {{ call.prompt_chars }} prompt · {{ call.response_chars }} response · {{ modelCallTokens(call) }} tokens
                </div>
                <el-tag v-if="modelCallSignal(call)" size="small" type="warning" class="trace-signal">
                  {{ modelCallSignal(call) }}
                </el-tag>
                <p v-if="call.last_error" class="muted danger trace-error">
                  {{ compactTraceText(call.last_error) }}
                </p>
              </div>
              <el-empty v-if="!traceModelCalls.length && hasTraceData" description="暂无 model calls" />
            </div>
            <div class="trace-column">
              <h3>Retrievals</h3>
              <div v-for="retrieval in traceRetrievals" :key="retrieval.id" class="trace-row">
                <div class="trace-row-main">
                  <span class="trace-title">{{ retrieval.retriever_type }}</span>
                  <el-tag size="small" :type="jobStatusType(retrieval.status)">
                    {{ traceStatusLabel(retrieval.status) }} ({{ retrieval.status }})
                  </el-tag>
                </div>
                <div class="muted small">
                  {{ retrieval.query_count }} queries · {{ retrieval.hit_count }} hits
                </div>
              </div>
              <el-empty v-if="!traceRetrievals.length && hasTraceData" description="暂无 retrieval runs" />
            </div>
            <div class="trace-column">
              <h3>Artifacts</h3>
              <div v-for="artifact in traceArtifacts" :key="artifact.id" class="trace-row">
                <div class="trace-row-main">
                  <span class="trace-title">{{ artifact.name || artifact.artifact_type }}</span>
                  <el-tag size="small" type="info">{{ artifact.artifact_type }}</el-tag>
                </div>
                <div class="muted small">
                  {{ artifactLabel(artifact) }} · {{ formatDate(artifact.created_at) }}
                </div>
              </div>
              <el-empty v-if="!traceArtifacts.length && hasTraceData" description="暂无 artifacts" />
            </div>
          </div>
        </el-collapse-item>
      </el-collapse>
    </el-card>

    <el-dialog
      v-model="provenanceVisible"
      :title="`用例依据: ${provenanceCase?.case_title || ''}`"
      width="88%"
    >
      <div v-if="provenanceCase" class="provenance-view">
        <el-tabs v-model="provenanceTab">
          <el-tab-pane label="依据内容" name="sources">
        <div class="provenance-tags">
          <el-tag size="small" type="info">{{ provenanceCase.section }}</el-tag>
          <el-tag size="small">case #{{ provenanceCase.case_index + 1 }}</el-tag>
          <el-tag
            v-for="(count, type) in provenanceCase.feedback_counts || {}"
            :key="type"
            size="small"
            type="warning"
          >
            {{ feedbackTypeLabel(type) }} {{ count }}
          </el-tag>
        </div>

        <div v-if="provenanceFeedbackRows().length" class="provenance-block">
          <h3>审核记录</h3>
          <div
            v-for="item in provenanceFeedbackRows()"
            :key="item.id"
            class="trace-row"
          >
            <div class="trace-row-main">
              <span class="trace-title">{{ feedbackTypeLabel(item.feedback_type) }}</span>
              <span class="muted small">{{ formatDate(item.created_at) }}</span>
            </div>
            <p v-if="item.note" class="trace-snippet">{{ item.note }}</p>
          </div>
        </div>

        <details class="source-ctx provenance-queries">
          <summary>检索关键词</summary>
          <div class="query-list">
            <el-tag
              v-for="query in provenanceArray('document_queries')"
              :key="`doc-${query}`"
              size="small"
              type="info"
            >doc: {{ query }}</el-tag>
            <el-tag
              v-for="query in provenanceArray('knowledge_queries')"
              :key="`kb-${query}`"
              size="small"
            >kb: {{ query }}</el-tag>
          </div>
        </details>

        <div class="provenance-grid">
          <div class="provenance-block">
            <h3>参考文档片段</h3>
            <div v-for="doc in provenanceArray('document_hits')" :key="`${doc.document_id}-${doc.rank}`" class="trace-row">
              <div class="trace-row-main">
                <span class="trace-title">{{ doc.name || `document #${doc.document_id}` }}</span>
              </div>
              <p
                v-for="chunk in doc.top_chunks || []"
                :key="`${chunk.rank}-${chunk.query}`"
                class="trace-snippet"
              >
                {{ chunk.text }}
              </p>
            </div>
            <el-empty v-if="!provenanceArray('document_hits').length" description="暂无文档命中" />
          </div>

          <div class="provenance-block">
            <h3>知识片段</h3>
            <div v-for="hit in provenanceArray('knowledge_hits')" :key="`${hit.id}-${hit.rank}`" class="trace-row">
              <div class="trace-row-main">
                <span class="trace-title">{{ hit.name || `knowledge #${hit.id}` }}</span>
                <el-tag size="small">{{ hit.type || 'knowledge' }}</el-tag>
              </div>
              <div class="query-list mini">
                <el-tag v-for="query in hit.hit_queries || []" :key="query" size="small" type="info">
                  {{ query }}
                </el-tag>
              </div>
              <p v-if="hit.content_preview" class="trace-snippet knowledge-preview">
                {{ hit.content_preview }}
              </p>
            </div>
            <el-empty v-if="!provenanceArray('knowledge_hits').length" description="暂无知识命中" />
          </div>
        </div>
          </el-tab-pane>

          <el-tab-pane label="技术详情" name="technical">
            <div class="provenance-tags">
              <el-tag size="small" type="success">{{ provenanceTokenTotal() }} tokens</el-tag>
            </div>
        <div class="provenance-grid">
          <div class="provenance-block">
            <h3>Agents</h3>
            <div v-for="agent in provenanceAgentRuns()" :key="agent.id" class="trace-row">
              <div class="trace-row-main">
                <span class="trace-title">{{ agent.agent || agent.agent_name }}</span>
                <el-tag size="small" :type="jobStatusType(agent.status)">
                  {{ traceStatusLabel(agent.status) }}
                </el-tag>
              </div>
              <div class="muted small">{{ agent.stage || agent.attempt || '-' }}</div>
            </div>
          </div>

          <div class="provenance-block">
            <h3>Model Calls</h3>
            <div v-for="call in provenanceModelCalls()" :key="call.id" class="trace-row">
              <div class="trace-row-main">
                <span class="trace-title">{{ call.provider || '-' }} / {{ call.model || '-' }}</span>
                <el-tag size="small" :type="jobStatusType(call.status)">
                  {{ traceStatusLabel(call.status) }}
                </el-tag>
              </div>
              <div class="muted small">
                #{{ call.id }} · {{ call.agent || call.metadata?.agent || '-' }} ·
                {{ call.prompt_id || call.metadata?.prompt_id || '-' }}@{{ call.prompt_version || call.metadata?.prompt_version || '-' }}
              </div>
              <div class="muted small">
                {{ call.prompt_chars }} prompt · {{ call.response_chars }} response · {{ modelCallTokens(call) }} tokens
              </div>
              <el-tag v-if="modelCallSignal(call)" size="small" type="warning" class="trace-signal">
                {{ modelCallSignal(call) }}
              </el-tag>
              <p v-if="call.last_error" class="muted danger trace-error">
                {{ compactTraceText(call.last_error) }}
              </p>
            </div>
          </div>
        </div>
          </el-tab-pane>
        </el-tabs>
      </div>
    </el-dialog>

    <el-dialog
      v-model="editorVisible"
      :title="`编辑 section: ${editingCase?.section || ''}`"
      width="780px"
      :close-on-click-modal="false"
    >
      <p class="muted small">
        直接编辑 JSON 数组（本 section 的全部 cases）。保存后整段替换；建议保留
        <code>title / priority_id / custom_preconds / custom_steps_separated</code> 四个字段。
      </p>
      <el-input
        v-model="editorBuffer"
        type="textarea"
        :rows="20"
        spellcheck="false"
        style="font-family: ui-monospace, Consolas, monospace"
      />
      <template #footer>
        <el-tag v-if="editorDirty" type="warning" size="small">未保存</el-tag>
        <el-button @click="editorVisible = false">取消</el-button>
        <el-button type="primary" :loading="casesSaving" @click="saveEditor">保存</el-button>
      </template>
    </el-dialog>

    <el-drawer
      v-model="caseEditorVisible"
      :title="`编辑用例: ${editingCaseSection?.section || ''}`"
      direction="rtl"
      size="640px"
      :close-on-click-modal="false"
    >
      <el-form label-width="88px" class="case-editor-form">
        <el-form-item label="标题">
          <el-input v-model="caseForm.title" maxlength="180" show-word-limit />
        </el-form-item>
        <el-form-item label="优先级">
          <el-select v-model="caseForm.priority_id" class="case-editor-control">
            <el-option
              v-for="item in priorityOptions"
              :key="item.value"
              :label="item.label"
              :value="item.value"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="影响产品">
          <el-select
            v-model="caseForm.affected_products"
            multiple
            filterable
            allow-create
            collapse-tags
            class="case-editor-control"
          >
            <el-option v-for="item in productOptions" :key="item" :label="item" :value="item" />
          </el-select>
        </el-form-item>
        <el-form-item label="影响模块">
          <el-select
            v-model="caseForm.affected_modules"
            multiple
            filterable
            allow-create
            collapse-tags
            class="case-editor-control"
          >
            <el-option v-for="item in moduleOptions" :key="item" :label="item" :value="item" />
          </el-select>
        </el-form-item>
        <el-form-item label="前置条件">
          <el-input
            v-model="caseForm.custom_preconds"
            type="textarea"
            :rows="4"
            maxlength="600"
            show-word-limit
          />
        </el-form-item>
        <el-form-item label="步骤">
          <div class="step-editor-list">
            <div
              v-for="(step, index) in caseForm.custom_steps_separated"
              :key="index"
              class="step-editor-row"
            >
              <span class="step-editor-index">{{ index + 1 }}</span>
              <div class="step-editor-fields">
                <el-input v-model="step.content" placeholder="操作" maxlength="500" />
                <el-input v-model="step.expected" placeholder="预期" maxlength="500" />
              </div>
              <el-button
                :icon="Delete"
                :aria-label="`删除第 ${index + 1} 个步骤`"
                circle
                text
                type="danger"
                @click="removeCaseStep(index)"
              />
            </div>
            <el-button :icon="Plus" @click="addCaseStep">添加步骤</el-button>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <div class="drawer-footer">
          <el-tag v-if="caseEditorDirty" type="warning" size="small">未保存</el-tag>
          <el-button @click="caseEditorVisible = false">取消</el-button>
          <el-button
            type="primary"
            :loading="casesSaving"
            :disabled="!caseForm.title.trim()"
            @click="saveCaseEditor"
          >保存</el-button>
        </div>
      </template>
    </el-drawer>

    <el-dialog
      v-model="qualityFeedbackVisible"
      title="用例质量反馈"
      width="560px"
      :close-on-click-modal="false"
    >
      <el-form label-width="100px">
        <el-form-item label="用例">
          <el-input v-model="qualityFeedbackForm.case_title" disabled />
        </el-form-item>
        <el-form-item label="反馈类型">
          <el-radio-group v-model="qualityFeedbackForm.feedback_type" class="feedback-type-group">
            <el-radio
              v-for="option in qualityFeedbackOptions"
              :key="option.value"
              :value="option.value"
            >
              {{ option.label }}
            </el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="备注">
          <el-input
            v-model="qualityFeedbackForm.note"
            type="textarea"
            :rows="4"
            maxlength="500"
            show-word-limit
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="qualityFeedbackVisible = false">取消</el-button>
        <el-button
          type="primary"
          :loading="feedbackSaving"
          @click="submitQualityFeedback"
        >提交</el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="feedbackVisible"
      title="反馈知识缺失"
      width="520px"
      :close-on-click-modal="false"
    >
      <el-form label-width="100px">
        <el-form-item label="类型">
          <el-radio-group v-model="feedbackForm.candidate_type">
            <el-radio value="product">{{ knowledgeTypeLabel('product') }}</el-radio>
            <el-radio value="module">{{ knowledgeTypeLabel('module') }}</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="候选名称">
          <el-input v-model="feedbackForm.candidate_name" maxlength="120" show-word-limit />
        </el-form-item>
        <el-form-item label="来源用例">
          <el-input v-model="feedbackForm.source_case_title" disabled />
        </el-form-item>
        <el-form-item label="备注">
          <el-input
            v-model="feedbackForm.note"
            type="textarea"
            :rows="4"
            maxlength="500"
            show-word-limit
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="feedbackVisible = false">取消</el-button>
        <el-button
          type="primary"
          :loading="suggestionStore.saving"
          @click="submitKnowledgeFeedback"
        >提交</el-button>
      </template>
    </el-dialog>
  </section>
  <el-empty v-else description="加载中..." />
</template>

<style scoped>
.task-detail {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.page-header {
  background: #fff;
  border-radius: 8px;
  padding: 16px 24px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
}
.task-meta {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-top: 12px;
}
.task-view-switch {
  display: flex;
  justify-content: flex-start;
}
.muted {
  color: #909399;
  font-size: 13px;
}
.danger {
  color: #f56c6c;
  margin-top: 12px;
}
.small {
  font-size: 12px;
}
.card {
  border-radius: 8px;
}
.readonly-scope {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 20px;
}
.readonly-scope > div {
  display: grid;
  grid-template-columns: 96px minmax(0, 1fr);
  gap: 10px;
  color: #606266;
  font-size: 14px;
}
.readonly-scope > div > div {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}
.diagnostics-card {
  overflow: hidden;
}
.diagnostic-summary {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-bottom: 10px;
}
.diagnostic-collapse {
  border-top: 1px solid #ebeef5;
}
.diagnostic-title {
  color: #303133;
  font-size: 13px;
  font-weight: 600;
}
.trace-summary-grid {
  display: grid;
  grid-template-columns: repeat(6, minmax(0, 1fr));
  gap: 8px;
  margin-bottom: 14px;
}
.trace-metric {
  min-width: 0;
  border: 1px solid #ebeef5;
  border-radius: 6px;
  padding: 10px 12px;
  background: #fafafa;
}
.metric-value {
  display: block;
  color: #303133;
  font-size: 20px;
  font-weight: 700;
  line-height: 1.2;
}
.metric-label {
  display: block;
  margin-top: 2px;
  color: #909399;
  font-size: 12px;
}
.trace-layout {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 12px;
}
.trace-column {
  min-width: 0;
}
.trace-column h3 {
  margin: 0 0 8px;
  color: #606266;
  font-size: 13px;
  font-weight: 600;
}
.trace-row {
  min-width: 0;
  border: 1px solid #ebeef5;
  border-radius: 6px;
  padding: 10px;
  margin-bottom: 8px;
  background: #fff;
}
.trace-row-main {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  min-width: 0;
}
.trace-title {
  min-width: 0;
  overflow: hidden;
  color: #303133;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.trace-snippet {
  margin: 6px 0 0;
  color: #606266;
  font-size: 12px;
  line-height: 1.5;
  word-break: break-word;
}
.knowledge-preview {
  white-space: pre-wrap;
}
.trace-error {
  margin-bottom: 8px;
  word-break: break-word;
}
.trace-signal {
  margin-top: 6px;
}
.provenance-view {
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.provenance-tags,
.query-list {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.query-list.mini {
  margin-top: 6px;
}
.provenance-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}
.provenance-block {
  min-width: 0;
}
.provenance-block h3 {
  margin: 0 0 8px;
  color: #606266;
  font-size: 13px;
  font-weight: 600;
}
.provenance-queries {
  margin-bottom: 12px;
}
.job-timeline {
  padding: 4px 0 0;
}
.job-row {
  min-width: 0;
}
.job-row-main {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.job-label {
  color: #303133;
  font-weight: 600;
}
.job-error {
  margin: 6px 0 0;
  word-break: break-word;
}
.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}
.header-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  justify-content: flex-end;
}
.section-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 500;
}
.section-actions {
  display: flex;
  gap: 8px;
  margin-bottom: 12px;
  flex-wrap: wrap;
}
.case-overview {
  margin-bottom: 14px;
  padding: 14px 16px 10px;
  border-top: 1px solid #ebeef5;
  border-bottom: 1px solid #ebeef5;
  background: #fafafa;
}
.overview-header,
.overview-section-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.overview-header h3 {
  margin: 0;
  font-size: 15px;
  letter-spacing: 0;
}
.overview-header p {
  margin: 3px 0 0;
}
.overview-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.overview-scroll {
  margin-top: 10px;
}
.overview-sections {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  column-gap: 24px;
}
.overview-section {
  min-width: 0;
  padding: 10px 0 12px;
  border-top: 1px solid #ebeef5;
}
.overview-title-list {
  margin: 8px 0 0;
  padding-left: 24px;
}
.overview-title-list li {
  padding: 1px 0;
  color: #909399;
}
.overview-title-list :deep(.el-button) {
  max-width: 100%;
  height: auto;
  padding: 3px 0;
  justify-content: flex-start;
  text-align: left;
  white-space: normal;
  line-height: 1.45;
}
.case-title-button {
  max-width: 100%;
  height: auto;
  padding: 4px 0;
  justify-content: flex-start;
  text-align: left;
  white-space: normal;
  line-height: 1.45;
}
.case-review-tools {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin-bottom: 12px;
}
.case-detail-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding-bottom: 10px;
  border-bottom: 1px solid #ebeef5;
  color: #606266;
  font-size: 13px;
  font-weight: 600;
}
.case-detail-toolbar > div {
  display: flex;
  align-items: center;
  gap: 8px;
}
.section-selection-label {
  color: #909399;
  font-weight: 400;
}
.filter-bar,
.batch-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.case-filter-bar {
  padding: 10px;
  background: #fafafa;
  border: 1px solid #ebeef5;
  border-radius: 6px;
}
.advanced-filter-bar {
  margin-top: -10px;
  border-top: 0;
  border-top-left-radius: 0;
  border-top-right-radius: 0;
}
.filter-control,
.batch-control {
  width: 180px;
}
.filter-control.compact,
.batch-control.compact {
  width: 128px;
}
.keyword-filter {
  width: 280px;
}
.batch-bar {
  min-height: 40px;
}
.case-editor-form {
  padding-right: 8px;
}
.case-editor-control {
  width: 100%;
}
.step-editor-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
  width: 100%;
}
.step-editor-row {
  display: grid;
  grid-template-columns: 28px minmax(0, 1fr) 32px;
  gap: 8px;
  align-items: start;
}
.step-editor-index {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  color: #606266;
  background: #f5f7fa;
  border-radius: 6px;
  font-size: 12px;
  font-weight: 600;
}
.step-editor-fields {
  display: grid;
  gap: 8px;
}
.drawer-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
.chip {
  margin-right: 4px;
}
.case-review-actions {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  white-space: nowrap;
}
.case-review-actions :deep(.el-button) {
  margin-left: 0;
}
.feedback-type-group {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 14px;
}
.source-ctx {
  margin-top: 12px;
  background: #fafafa;
  padding: 12px;
  border-radius: 4px;
  font-size: 12px;
}
.source-ctx pre {
  white-space: pre-wrap;
  word-break: break-all;
  margin: 8px 0 0;
  max-height: 320px;
  overflow: auto;
}
.case-expand {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 4px 16px 12px 48px;
}
.case-field {
  display: grid;
  grid-template-columns: 88px minmax(0, 1fr);
  gap: 12px;
  align-items: start;
}
.field-label {
  color: #606266;
  font-size: 13px;
  font-weight: 500;
}
.field-body {
  color: #303133;
  line-height: 1.6;
  word-break: break-word;
}
.steps-table {
  width: 100%;
}
@media (max-width: 1200px) {
  .trace-summary-grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
  .trace-layout {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .provenance-grid {
    grid-template-columns: 1fr;
  }
}
@media (max-width: 720px) {
  .task-meta,
  .card-header,
  .header-actions,
  .filter-bar,
  .batch-bar {
    align-items: flex-start;
    flex-direction: column;
  }
  .filter-control,
  .batch-control,
  .filter-control.compact,
  .batch-control.compact {
    width: 100%;
  }
  .trace-summary-grid,
  .trace-layout,
  .provenance-grid,
  .overview-sections {
    grid-template-columns: 1fr;
  }
  .overview-header,
  .overview-section-header {
    align-items: flex-start;
  }
  .overview-header {
    flex-direction: column;
  }
  .case-overview {
    padding-inline: 12px;
  }
}
</style>
