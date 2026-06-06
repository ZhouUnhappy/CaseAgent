<script setup>
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { storeToRefs } from 'pinia'
import { ElMessageBox } from 'element-plus'
import { ChatDotRound, Check, Delete, Download, EditPen, Plus, Refresh, View, Warning } from '@element-plus/icons-vue'
import StatusTag from '../components/StatusTag.vue'
import { listJobs } from '../api/jobs'
import { getTaskTrace } from '../api/tasks'
import { useTasksStore } from '../stores/tasks'
import { useTestCasesStore } from '../stores/testcases'
import { useKnowledgeStore } from '../stores/knowledge'
import { useKnowledgeSuggestionsStore } from '../stores/knowledgeSuggestions'
import { notifySuccess } from '../utils/error'
import { compactJobError, jobStatusLabel, jobStatusType, jobTypeLabel } from '../utils/jobs'

const route = useRoute()
const router = useRouter()
const taskId = computed(() => Number(route.params.id))

const tasksStore = useTasksStore()
const casesStore = useTestCasesStore()
const knowledgeStore = useKnowledgeStore()
const suggestionStore = useKnowledgeSuggestionsStore()

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
const editingCase = ref(null)
const editorVisible = ref(false)
const editorBuffer = ref('')
const provenanceVisible = ref(false)
const provenanceCase = ref(null)
const selectedCaseRefs = ref({})
const caseEditorVisible = ref(false)
const editingCaseSection = ref(null)
const editingCaseIndex = ref(-1)
const caseForm = reactive({
  title: '',
  priority_id: 2,
  custom_preconds: '',
  affected_products: [],
  affected_modules: [],
  custom_steps_separated: [],
})
const caseFilters = reactive({
  section: '',
  priority_id: '',
  product: '',
  module: '',
  feedback_type: '',
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
const hasActiveCaseFilters = computed(() =>
  Boolean(
    caseFilters.section ||
      caseFilters.priority_id ||
      caseFilters.product ||
      caseFilters.module ||
      caseFilters.feedback_type ||
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
      label: 'Created',
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
      label: 'Review',
      status: task.value.status === 'awaiting_review' ? 'running' : 'succeeded',
      time: task.value.updated_at,
    })
  }
  if (['completed', 'failed'].includes(task.value.status)) {
    rows.push({
      key: 'final',
      label: task.value.status === 'completed' ? 'Completed' : 'Failed',
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
  await loadTask()
  knowledgeStore.fetch().catch(() => {})
})
onUnmounted(() => stopPolling())

watch(taskId, () => {
  stopPolling()
  casesStore.clear()
  clearSelectedCases()
  loadTask()
})

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
      refreshCases().catch(() => {})
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

function formatDate(value) {
  return value ? new Date(value).toLocaleString() : '-'
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

function caseFeedbackCount(section, row, index) {
  return feedbackCountTotal(findCaseProvenance(section, row, index)?.feedback_counts)
}

function caseFeedbackCounts(section, row, index) {
  return findCaseProvenance(section, row, index)?.feedback_counts || {}
}

function matchesCaseFilters(section, row, index) {
  if (caseFilters.hide_duplicates && row.duplicate_hidden) return false
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

function resetCaseFilters() {
  Object.assign(caseFilters, {
    section: '',
    priority_id: '',
    product: '',
    module: '',
    feedback_type: '',
    provenance: '',
    hide_duplicates: true,
  })
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

function selectedTestCaseIDs() {
  return [...new Set(selectedCases.value.map((item) => item.test_case_id))]
}

function clearSelectedCases() {
  selectedCaseRefs.value = {}
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
  const blob = new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = `caseagent-task-${taskId.value}-test-cases.json`
  document.body.appendChild(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
}

function openEditor(section) {
  editingCase.value = section
  editorBuffer.value = JSON.stringify(section.cases || [], null, 2)
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
  try {
    await casesStore.update(taskId.value, editingCase.value.id, {
      section: editingCase.value.section,
      cases: parsed,
    })
    notifySuccess('用例已保存')
    editorVisible.value = false
  } catch {
    /* 错误已弹窗 */
  }
}

async function submitSection(section) {
  try {
    await ElMessageBox.confirm(
      `提交 section「${section.section}」共 ${section.cases?.length || 0} 条用例？`,
      '确认',
      { type: 'info' },
    )
  } catch {
    return
  }
  try {
    await casesStore.submit(taskId.value, section.id)
    notifySuccess('已提交')
  } catch {
    /* 错误已弹窗 */
  }
}

async function submitSelectedSections() {
  const ids = selectedTestCaseIDs()
  if (!ids.length) {
    ElMessageBox.alert('请先选择用例')
    return
  }
  try {
    await ElMessageBox.confirm(`提交已选用例所属的 ${ids.length} 个 section？`, '确认', { type: 'info' })
  } catch {
    return
  }
  try {
    await casesStore.batchSubmit(taskId.value, ids)
    clearSelectedCases()
    notifySuccess('已批量提交')
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
    await casesStore.batchUpdate(taskId.value, {
      cases: selectedCases.value.map(({ test_case_id, case_index }) => ({ test_case_id, case_index })),
      patch: { duplicate_hidden: true },
    })
    clearSelectedCases()
    notifySuccess('已标记重复并隐藏')
  } catch {
    /* 错误已弹窗 */
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

async function saveCaseEditor() {
  if (!editingCaseSection.value || editingCaseIndex.value < 0) return
  const nextCases = (editingCaseSection.value.cases || []).map((row) => ({ ...row }))
  nextCases[editingCaseIndex.value] = {
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
  try {
    await casesStore.update(taskId.value, editingCaseSection.value.id, {
      section: editingCaseSection.value.section,
      cases: nextCases,
    })
    notifySuccess('用例已保存')
    caseEditorVisible.value = false
  } catch {
    /* 错误已弹窗 */
  }
}

function openQualityFeedback(section, row, index) {
  Object.assign(qualityFeedbackForm, {
    test_case_id: section.id,
    case_index: index,
    case_title: row.title || section.section || '',
    feedback_type: 'useful',
    note: '',
  })
  qualityFeedbackVisible.value = true
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
    feedbackVisible.value = false
    notifySuccess('知识缺失反馈已提交')
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
        点击"重试"会根据当前状态自动回到 analyze 或 ready_to_generate；若回到 ready_to_generate，
        请确认根因（后端日志关键字 <code>agent</code> / <code>document</code>）已修复后再"开始生成"。
      </p>
    </header>

    <el-card shadow="never" class="card">
      <template #header><span>影响范围审核</span></template>
      <el-form label-width="100px">
        <el-form-item label="受影响产品">
          <el-select
            v-model="reviewForm.products"
            multiple
            filterable
            allow-create
            :disabled="!canReview"
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
            :disabled="!canReview"
            style="width: 100%"
            placeholder="留空表示不限定"
          >
            <el-option v-for="m in moduleOptions" :key="m" :value="m" :label="m" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button
            type="primary"
            :disabled="!canReview"
            @click="submitReview"
          >提交审核</el-button>
          <el-button
            type="success"
            :disabled="!canGenerate"
            :loading="generating"
            @click="startGenerate"
          >开始生成</el-button>
          <span v-if="!canReview && !canGenerate" class="muted small">
            当前状态 {{ task.status }} 不允许编辑影响范围或触发生成。
          </span>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card shadow="never" class="card case-output-card">
      <template #header>
        <div class="card-header">
          <span>测试用例输出</span>
          <div class="header-actions">
            <el-tag type="info" size="small">{{ caseSectionCount }} sections</el-tag>
            <el-tag type="success" size="small">{{ totalCaseCount }} cases</el-tag>
            <el-tag v-if="filteredCaseCount !== totalCaseCount" type="warning" size="small">
              {{ filteredCaseCount }} visible
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
        <div class="case-review-tools">
          <div class="filter-bar case-filter-bar">
            <el-select v-model="caseFilters.section" clearable placeholder="section" class="filter-control">
              <el-option v-for="item in sectionOptions" :key="item" :label="item" :value="item" />
            </el-select>
            <el-select v-model="caseFilters.priority_id" clearable placeholder="priority" class="filter-control compact">
              <el-option v-for="item in priorityOptions" :key="item.value" :label="item.label" :value="item.value" />
            </el-select>
            <el-select v-model="caseFilters.product" clearable filterable placeholder="product" class="filter-control">
              <el-option v-for="item in productOptions" :key="item" :label="item" :value="item" />
            </el-select>
            <el-select v-model="caseFilters.module" clearable filterable placeholder="module" class="filter-control">
              <el-option v-for="item in moduleOptions" :key="item" :label="item" :value="item" />
            </el-select>
            <el-select v-model="caseFilters.feedback_type" clearable placeholder="feedback" class="filter-control">
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
            <el-button @click="resetCaseFilters">重置</el-button>
          </div>

          <div class="batch-bar">
            <el-tag type="info" size="small">{{ selectedCaseCount }} selected</el-tag>
            <el-select v-model="batchPatch.priority_id" clearable placeholder="priority" class="batch-control compact">
              <el-option v-for="item in priorityOptions" :key="item.value" :label="item.label" :value="item.value" />
            </el-select>
            <el-select
              v-model="batchPatch.affected_products"
              multiple
              filterable
              allow-create
              collapse-tags
              placeholder="products"
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
              placeholder="modules"
              class="batch-control"
            >
              <el-option v-for="item in moduleOptions" :key="item" :label="item" :value="item" />
            </el-select>
            <el-button :icon="EditPen" :loading="batchSaving" @click="applyBatchPatch">批量修改</el-button>
            <el-button :icon="Warning" :loading="batchSaving" @click="hideSelectedDuplicates">标记重复</el-button>
            <el-button type="primary" :icon="Check" :loading="batchSaving" @click="submitSelectedSections">批量提交</el-button>
          </div>
        </div>
        <el-empty
          v-if="!filteredCaseCount && !casesLoading"
          description="没有符合筛选条件的用例"
        />
        <el-collapse v-else>
          <el-collapse-item
            v-for="section in filteredSections"
            :key="section.id"
            :name="section.id"
          >
          <template #title>
            <div class="section-title">
              <span>{{ section.section }}</span>
              <el-tag size="small" type="info">
                {{ section.display_cases?.length || 0 }}/{{ section.cases?.length || 0 }} cases
              </el-tag>
              <StatusTag :status="section.status" />
            </div>
          </template>

          <div class="section-actions">
            <el-button size="small" :icon="EditPen" @click="openEditor(section)">编辑 JSON</el-button>
            <el-button
              size="small"
              type="primary"
              :icon="Check"
              :disabled="section.status === 'submitted' || section.status === 'approved'"
              @click="submitSection(section)"
            >提交</el-button>
          </div>

          <el-table
            :data="section.display_cases || []"
            stripe
            size="small"
            @selection-change="(selection) => onCaseSelectionChange(section, selection)"
          >
            <el-table-column type="selection" width="44" />
            <el-table-column type="expand" width="48">
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
            <el-table-column prop="title" label="标题" min-width="260" show-overflow-tooltip />
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
                >调试</el-button>
              </template>
            </el-table-column>
            <el-table-column label="反馈" width="190" align="center">
              <template #default="{ row }">
                <div class="case-feedback-actions">
                  <el-button
                    size="small"
                    :icon="ChatDotRound"
                    @click="openQualityFeedback(section, row, row.__case_index)"
                  >
                    质量
                    <span v-if="caseFeedbackCount(section, row, row.__case_index)">
                      ({{ caseFeedbackCount(section, row, row.__case_index) }})
                    </span>
                  </el-button>
                  <el-button
                    size="small"
                    :icon="Warning"
                    @click="openKnowledgeFeedback(section, row)"
                  >
                    知识
                  </el-button>
                </div>
              </template>
            </el-table-column>
          </el-table>

          <details v-if="section.source_context" class="source-ctx">
            <summary>source_context 追溯（{{
              (section.source_context.knowledge_shipped_ids || []).length
            }} knowledge ids,
            {{ (section.source_context.document_hits || []).length }} doc hits）</summary>
            <pre>{{ JSON.stringify(section.source_context, null, 2) }}</pre>
          </details>
          </el-collapse-item>
        </el-collapse>
      </template>
    </el-card>

    <el-card shadow="never" class="card diagnostics-card" v-loading="traceLoading">
      <template #header>
        <div class="card-header">
          <span>诊断信息</span>
          <div class="header-actions">
            <el-button size="small" :loading="jobsLoading" @click="loadJobs">刷新任务</el-button>
            <el-button size="small" :loading="traceLoading" @click="loadTrace">刷新 Trace</el-button>
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
                    {{ jobStatusLabel(item.status) }}
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
                  <span class="trace-title">{{ jobTypeLabel(run.workflow_type) }}</span>
                  <el-tag size="small" :type="jobStatusType(run.status)">
                    {{ traceStatusLabel(run.status) }}
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
                    {{ traceStatusLabel(agent.status) }}
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
                    {{ traceStatusLabel(call.status) }}
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
                    {{ traceStatusLabel(retrieval.status) }}
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
      :title="`生成依据: ${provenanceCase?.case_title || ''}`"
      width="860px"
    >
      <div v-if="provenanceCase" class="provenance-view">
        <div class="provenance-tags">
          <el-tag size="small" type="info">{{ provenanceCase.section }}</el-tag>
          <el-tag size="small">case #{{ provenanceCase.case_index + 1 }}</el-tag>
          <el-tag size="small" type="success">{{ provenanceTokenTotal() }} tokens</el-tag>
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
          <h3>Feedback</h3>
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

        <div class="provenance-block">
          <h3>Queries</h3>
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
        </div>

        <div class="provenance-grid">
          <div class="provenance-block">
            <h3>Documents</h3>
            <div v-for="doc in provenanceArray('document_hits')" :key="`${doc.document_id}-${doc.rank}`" class="trace-row">
              <div class="trace-row-main">
                <span class="trace-title">{{ doc.name || `document #${doc.document_id}` }}</span>
                <el-tag size="small" type="info">rank {{ doc.rank }}</el-tag>
              </div>
              <div class="muted small">score {{ doc.best_score ?? '-' }}</div>
              <p
                v-for="chunk in doc.top_chunks || []"
                :key="`${chunk.rank}-${chunk.query}`"
                class="trace-snippet"
              >
                [{{ chunk.rank }} · {{ chunk.score }}] {{ chunk.text }}
              </p>
            </div>
            <el-empty v-if="!provenanceArray('document_hits').length" description="暂无文档命中" />
          </div>

          <div class="provenance-block">
            <h3>Knowledge</h3>
            <div v-for="hit in provenanceArray('knowledge_hits')" :key="`${hit.id}-${hit.rank}`" class="trace-row">
              <div class="trace-row-main">
                <span class="trace-title">{{ hit.name || `knowledge #${hit.id}` }}</span>
                <el-tag size="small">{{ hit.type || 'knowledge' }}</el-tag>
              </div>
              <div class="muted small">rank {{ hit.rank }} · score {{ hit.score ?? '-' }}</div>
              <div class="query-list mini">
                <el-tag v-for="query in hit.hit_queries || []" :key="query" size="small" type="info">
                  {{ query }}
                </el-tag>
              </div>
            </div>
            <el-empty v-if="!provenanceArray('knowledge_hits').length" description="暂无知识命中" />
          </div>
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
            <el-radio value="product">product</el-radio>
            <el-radio value="module">module</el-radio>
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
}
.case-review-tools {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin-bottom: 12px;
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
.filter-control,
.batch-control {
  width: 180px;
}
.filter-control.compact,
.batch-control.compact {
  width: 128px;
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
.case-feedback-actions {
  display: flex;
  justify-content: center;
  gap: 6px;
}
.case-feedback-actions :deep(.el-button) {
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
  .provenance-grid {
    grid-template-columns: 1fr;
  }
}
</style>
