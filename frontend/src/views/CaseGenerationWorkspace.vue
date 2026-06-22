<script setup>
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { storeToRefs } from 'pinia'
import { ElMessageBox } from 'element-plus'
import { Download, Plus, Refresh, Upload, VideoPlay, View } from '@element-plus/icons-vue'
import MarkdownPreview from '../components/MarkdownPreview.vue'
import StatusTag from '../components/StatusTag.vue'
import { listJobs } from '../api/jobs'
import { useProjectsStore } from '../stores/projects'
import { useDocumentsStore } from '../stores/documents'
import { useTasksStore } from '../stores/tasks'
import { useTestCasesStore } from '../stores/testcases'
import { useKnowledgeStore } from '../stores/knowledge'
import { formatDateTime as formatDate } from '../utils/date'
import { notifySuccess } from '../utils/error'
import { compactJobError, jobStatusLabel, jobStatusType, jobTypeLabel, latestJob } from '../utils/jobs'
import { statusLabel } from '../utils/status'

const route = useRoute()
const router = useRouter()

const projectsStore = useProjectsStore()
const documentsStore = useDocumentsStore()
const tasksStore = useTasksStore()
const casesStore = useTestCasesStore()
const knowledgeStore = useKnowledgeStore()

const { items: projects, loading: projectsLoading, creating: projectCreating } = storeToRefs(projectsStore)
const { items: documents, loading: documentsLoading, uploading } = storeToRefs(documentsStore)
const { items: tasks, loading: tasksLoading, creating: taskCreating, current: loadedTask } = storeToRefs(tasksStore)
const { items: cases, loading: casesLoading } = storeToRefs(casesStore)
const { items: knowledge } = storeToRefs(knowledgeStore)

const selectedProjectId = ref(null)
const selectedTaskId = ref(null)
const selectedDocumentIds = ref([])
const polling = ref(null)
const generating = ref(false)
const retrying = ref(false)
const taskJobs = ref({})
const selectedTaskJobs = ref([])

const createProjectDialog = ref(false)
const projectForm = reactive({ name: '', description: '' })

const uploadDialog = ref(false)
const uploadForm = reactive({ name: '', file: null })
const previewVisible = ref(false)
const previewDocument = ref(null)

const reviewForm = reactive({ products: [], modules: [] })

const task = computed(() => {
  if (loadedTask.value?.id === selectedTaskId.value) return loadedTask.value
  return tasks.value.find((item) => item.id === selectedTaskId.value) || null
})
const taskDocumentIds = computed(() => task.value?.document_ids || [])
const taskDocuments = computed(() =>
  taskDocumentIds.value.map((id) => documents.value.find((doc) => doc.id === id) || { id, name: `文档 #${id}` }),
)
const documentSelectionMatchesTask = computed(() => {
  if (!task.value) return true
  const selected = [...selectedDocumentIds.value].sort((a, b) => a - b)
  const taskInput = [...taskDocumentIds.value].sort((a, b) => a - b)
  return selected.length === taskInput.length && selected.every((id, index) => id === taskInput[index])
})

const completedDocuments = computed(() => documents.value.filter((doc) => doc.status === 'completed'))
const latestCompletedTask = computed(() => tasks.value.find((item) => item.status === 'completed') || null)
const productOptions = computed(() => knowledge.value.filter((k) => k.type === 'product').map((k) => k.name))
const moduleOptions = computed(() => knowledge.value.filter((k) => k.type === 'module').map((k) => k.name))
const canReview = computed(() => task.value && ['awaiting_review', 'ready_to_generate'].includes(task.value.status))
const canGenerate = computed(() => task.value?.status === 'ready_to_generate')
const canRetry = computed(() => task.value?.status === 'failed')
const isPolling = computed(() => task.value && ['analyzing', 'generating'].includes(task.value.status))
const caseSectionCount = computed(() => cases.value.length)
const totalCaseCount = computed(() =>
  cases.value.reduce((sum, section) => sum + (section.cases?.length || 0), 0),
)
const taskStatusText = computed(() => {
  if (!task.value) return '未创建任务'
  const job = latestSelectedJob.value
  if (job) return `${jobTypeLabel(job.job_type)} ${jobStatusLabel(job.status)}`
  return statusLabel(task.value.status)
})
const latestSelectedJob = computed(() => latestJob(selectedTaskJobs.value))
const selectedJobError = computed(() => compactJobError(findSelectedJobWithError()))
const showWorkspaceDiagnostics = computed(() => {
  const taskStatus = task.value?.status || ''
  const jobStatus = latestSelectedJob.value?.status || ''
  return ['analyzing', 'generating', 'failed'].includes(taskStatus) ||
    ['pending', 'retrying', 'running', 'failed', 'canceled'].includes(jobStatus)
})
const workspaceTimeline = computed(() => {
  if (!task.value) return []
  const rows = [
    {
      key: 'created',
      label: '任务已创建',
      status: 'succeeded',
      time: task.value.created_at,
    },
  ]
  for (const job of selectedTaskJobs.value) {
    rows.push({
      key: `job-${job.id}`,
      label: jobTypeLabel(job.job_type),
      status: job.status,
      time: job.started_at || job.run_after || job.created_at,
      retry: `${job.retry_count}/${job.max_retries}`,
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
    })
  }
  return rows
})
const flowSteps = computed(() => {
  const status = task.value?.status || ''
  const hasDocuments = completedDocuments.value.length > 0
  const hasGenerated = ['completed'].includes(status)
  return [
    {
      label: '参考文档',
      value: `${completedDocuments.value.length} 可用`,
      state: hasDocuments ? 'done' : selectedProjectId.value ? 'active' : 'waiting',
    },
    {
      label: '范围与生成',
      value: task.value ? statusLabel(status) : '未创建任务',
      state: hasGenerated ? 'done' : task.value ? 'active' : 'waiting',
    },
    {
      label: '输出',
      value: caseSectionCount.value ? `${caseSectionCount.value} 个类别` : '等待',
      state: caseSectionCount.value ? 'done' : 'waiting',
    },
  ]
})
const emptyCaseDescription = computed(() => {
  switch (task.value?.status) {
    case 'analyzing':
      return '正在分析影响范围'
    case 'awaiting_review':
      return '确认影响范围后即可生成'
    case 'ready_to_generate':
      return '点击开始生成后会在这里展示结果'
    case 'generating':
      return '正在生成测试用例'
    case 'failed':
      return '当前任务没有可展示的用例'
    default:
      return '选择一个已完成任务或创建新任务'
  }
})

onMounted(async () => {
  await Promise.all([
    projectsStore.fetch().catch(() => {}),
    knowledgeStore.fetch().catch(() => {}),
  ])

  const queryProjectID = Number(route.query.project_id)
  const initialProject = projects.value.find((project) => project.id === queryProjectID) || projects.value[0]
  if (initialProject) {
    selectedProjectId.value = initialProject.id
  }
})

onUnmounted(() => stopPolling())

watch(selectedProjectId, async (projectID) => {
  stopPolling()
  selectedTaskId.value = null
  selectedDocumentIds.value = []
  selectedTaskJobs.value = []
  taskJobs.value = {}
  casesStore.clear()

  if (!projectID) return
  await Promise.all([
    documentsStore.fetch(projectID).catch(() => {}),
    tasksStore.fetch(projectID).catch(() => {}),
  ])
  loadTaskJobs().catch(() => {})

  selectedDocumentIds.value = completedDocuments.value.map((doc) => doc.id)

  const queryTaskID = Number(route.query.task_id)
  const initialTask = tasks.value.find((item) => item.id === queryTaskID) || latestCompletedTask.value || tasks.value[0]
  if (initialTask) {
    selectTask(initialTask)
  }
})

watch(
  () => task.value?.status,
  (status) => {
    if (status === 'completed' || status === 'failed') {
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

async function refreshProjectData() {
  if (!selectedProjectId.value) return
  const requests = [
    documentsStore.fetch(selectedProjectId.value).catch(() => {}),
    tasksStore.fetch(selectedProjectId.value).catch(() => {}),
  ]
  if (selectedTaskId.value) {
    requests.push(
      tasksStore.load(selectedTaskId.value).catch(() => {}),
      casesStore.fetch(selectedTaskId.value).catch(() => {}),
    )
  }
  await Promise.all(requests)
  await Promise.all([
    loadTaskJobs().catch(() => {}),
    loadSelectedTaskJobs(selectedTaskId.value).catch(() => {}),
  ])
}

function openCreateProject() {
  Object.assign(projectForm, { name: '', description: '' })
  createProjectDialog.value = true
}

async function submitCreateProject() {
  if (!projectForm.name.trim()) {
    ElMessageBox.alert('请输入项目名称')
    return
  }
  try {
    const created = await projectsStore.create({
      name: projectForm.name.trim(),
      description: projectForm.description.trim(),
    })
    createProjectDialog.value = false
    selectedProjectId.value = created.id
    notifySuccess('项目已创建')
  } catch {
    /* 错误已弹窗 */
  }
}

function openUpload() {
  Object.assign(uploadForm, { name: '', file: null })
  uploadDialog.value = true
}

function openDocumentPreview(doc) {
  if (doc.status !== 'completed' || !doc.content) return
  previewDocument.value = doc
  previewVisible.value = true
}

function onFileChange(file) {
  uploadForm.file = file?.raw || null
  if (!uploadForm.name && file?.name) {
    uploadForm.name = file.name
  }
}

async function submitUpload() {
  if (!selectedProjectId.value) return
  if (!uploadForm.file) {
    ElMessageBox.alert('请选择文件')
    return
  }
  if (!uploadForm.name.trim()) {
    ElMessageBox.alert('请输入文档名称')
    return
  }
  try {
    await documentsStore.upload(selectedProjectId.value, {
      name: uploadForm.name.trim(),
      type: 'markdown',
      source: 'upload',
      file: uploadForm.file,
    })
    uploadDialog.value = false
    notifySuccess('文档已上传，处理完成后即可生成用例')
  } catch {
    /* 错误已弹窗 */
  }
}

async function createTaskFromDocuments() {
  if (!selectedProjectId.value) return
  if (selectedDocumentIds.value.length === 0) {
    ElMessageBox.alert('请选择至少一份已完成处理的参考文档')
    return
  }
  try {
    const created = await tasksStore.create(selectedProjectId.value, {
      document_ids: selectedDocumentIds.value,
    })
    selectTask(created)
    notifySuccess('任务已创建，正在分析影响范围')
    ensurePolling()
  } catch {
    /* 错误已弹窗 */
  }
}

async function selectTask(row) {
  selectedTaskId.value = row.id
  loadSelectedTaskJobs(row.id).catch(() => {})
  try {
    const loaded = await tasksStore.load(row.id)
    selectedDocumentIds.value = [...(loaded.document_ids || [])]
    reviewForm.products = [...(loaded.affected_products || [])]
    reviewForm.modules = [...(loaded.affected_modules || [])]
    if (['completed', 'failed'].includes(loaded.status)) {
      refreshCases().catch(() => {})
    } else {
      casesStore.clear()
    }
  } catch {
    /* 错误已弹窗 */
  }
}

async function submitReview() {
  if (!task.value) return
  try {
    const updated = await tasksStore.review(task.value.id, {
      affected_products: reviewForm.products,
      affected_modules: reviewForm.modules,
    })
    selectTask(updated)
    notifySuccess('影响范围已确认')
  } catch {
    /* 错误已弹窗 */
  }
}

async function startGenerate() {
  if (!task.value) return
  generating.value = true
  try {
    const updated = await tasksStore.generate(task.value.id)
    selectTask(updated)
    notifySuccess('生成已触发')
    ensurePolling()
  } catch {
    /* 错误已弹窗 */
  } finally {
    generating.value = false
  }
}

async function retryTask() {
  if (!task.value) return
  retrying.value = true
  try {
    const updated = await tasksStore.retry(task.value.id)
    selectTask(updated)
    if (updated.status === 'analyzing') {
      notifySuccess('已重新触发分析')
      ensurePolling()
    } else {
      notifySuccess('任务已回到可生成状态')
    }
  } catch {
    /* 错误已弹窗 */
  } finally {
    retrying.value = false
  }
}

async function refreshCases() {
  if (!selectedTaskId.value) return
  await casesStore.fetch(selectedTaskId.value)
}

function ensurePolling() {
  if (polling.value || !selectedTaskId.value) return
  polling.value = setInterval(() => {
    tasksStore.load(selectedTaskId.value).catch(() => {})
    loadSelectedTaskJobs(selectedTaskId.value).catch(() => {})
  }, 3000)
}

function stopPolling() {
  if (!polling.value) return
  clearInterval(polling.value)
  polling.value = null
}

function exportCases() {
  if (!cases.value.length) return
  const payload = {
    task_id: selectedTaskId.value,
    task_status: task.value?.status || '',
    exported_at: new Date().toISOString(),
    sections: cases.value,
  }
  const blob = new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = `caseagent-task-${selectedTaskId.value}-test-cases.json`
  document.body.appendChild(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
}

function openTaskDetail() {
  if (!selectedTaskId.value) return
  router.push({ name: 'task-detail', params: { id: selectedTaskId.value } })
}

function priorityLabel(id) {
  return { 1: 'Low', 2: 'Medium', 3: 'High', 4: 'Critical' }[id] || `P${id}`
}

async function loadTaskJobs() {
  const entries = await Promise.all(
    tasks.value.map(async (row) => [row.id, latestJob(await listJobs({ task_id: row.id }))]),
  )
  taskJobs.value = Object.fromEntries(entries.filter(([, job]) => job))
}

async function loadSelectedTaskJobs(taskID) {
  if (!taskID) return
  selectedTaskJobs.value = await listJobs({ task_id: taskID })
  taskJobs.value = {
    ...taskJobs.value,
    [taskID]: latestJob(selectedTaskJobs.value),
  }
}

function latestTaskJob(row) {
  return taskJobs.value[row.id] || null
}

function findSelectedJobWithError() {
  for (let i = selectedTaskJobs.value.length - 1; i >= 0; i -= 1) {
    if (selectedTaskJobs.value[i].last_error) return selectedTaskJobs.value[i]
  }
  return null
}
</script>

<template>
  <section class="generate-workspace">
    <div class="workspace-head">
      <h1>生成测试用例</h1>
      <div class="head-actions">
        <el-button
          :icon="Refresh"
          :loading="documentsLoading || tasksLoading || casesLoading"
          :disabled="!selectedProjectId"
          @click="refreshProjectData"
        >
          刷新
        </el-button>
        <el-button v-if="projects.length" type="primary" :icon="Plus" @click="openCreateProject">
          新建项目
        </el-button>
      </div>
    </div>

    <div class="context-strip">
      <div class="project-select">
        <span>当前项目</span>
        <el-select
          v-model="selectedProjectId"
          :loading="projectsLoading"
          filterable
          placeholder="选择项目"
          class="project-control"
        >
          <el-option
            v-for="project in projects"
            :key="project.id"
            :label="project.name"
            :value="project.id"
          />
        </el-select>
      </div>
      <div class="flow-strip">
        <div
          v-for="step in flowSteps"
          :key="step.label"
          class="flow-step"
          :class="step.state"
        >
          <span class="flow-dot" />
          <div>
            <strong>{{ step.label }}</strong>
            <span>{{ step.value }}</span>
          </div>
        </div>
      </div>
    </div>

    <el-empty
      v-if="!projects.length && !projectsLoading"
      description="暂无项目，先新建一个项目"
    >
      <el-button type="primary" :icon="Plus" @click="openCreateProject">新建项目</el-button>
    </el-empty>

    <div v-else class="workspace-grid">
      <div class="workspace-side">
        <section class="panel document-panel">
          <header class="panel-head">
            <div>
              <span class="step-index">1</span>
              <h2>参考文档</h2>
            </div>
            <el-button type="primary" :icon="Upload" @click="openUpload" :disabled="!selectedProjectId">
              上传
            </el-button>
          </header>

          <el-alert
            v-if="documents.some((doc) => doc.status === 'processing')"
            title="有文档正在处理，完成后即可选中生成。"
            type="warning"
            :closable="false"
            class="inline-alert"
          />

          <el-checkbox-group v-model="selectedDocumentIds" class="doc-list">
            <div
              v-for="doc in documents"
              :key="doc.id"
              class="doc-row"
              :class="{ disabled: doc.status !== 'completed' }"
            >
              <el-checkbox
                :value="doc.id"
                :disabled="doc.status !== 'completed'"
                class="doc-check"
              />
              <div class="doc-main">
                <button
                  type="button"
                  class="doc-preview"
                  :disabled="doc.status !== 'completed' || !doc.content"
                  :title="doc.status === 'completed' && doc.content ? `预览 ${doc.name}` : '文档处理完成后可预览'"
                  @click="openDocumentPreview(doc)"
                >
                  {{ doc.name }}
                </button>
                <small>#{{ doc.id }} · {{ formatDate(doc.updated_at) }}</small>
              </div>
              <StatusTag class="doc-status" :status="doc.status" />
            </div>
          </el-checkbox-group>

          <el-empty v-if="!documents.length && !documentsLoading" description="暂无参考文档" />

          <div class="panel-footer">
            <span>{{ completedDocuments.length }} 份文档可用于生成</span>
            <el-button
              type="primary"
              :icon="Plus"
              :loading="taskCreating"
              :disabled="!selectedProjectId || selectedDocumentIds.length === 0"
              @click="createTaskFromDocuments"
            >
              创建生成任务
            </el-button>
          </div>
        </section>

        <section class="panel task-panel">
          <header class="panel-head">
            <div>
              <span class="step-index">2</span>
              <h2>影响范围与生成</h2>
            </div>
            <StatusTag v-if="task" :status="task.status" />
            <el-tag v-else type="info">{{ taskStatusText }}</el-tag>
          </header>

          <div class="task-list">
            <button
              v-for="item in tasks"
              :key="item.id"
              class="task-row"
              :class="{ active: item.id === selectedTaskId }"
              @click="selectTask(item)"
            >
              <div class="task-row-main">
                <span>任务 #{{ item.id }}</span>
                <small v-if="latestTaskJob(item)">
                  {{ jobTypeLabel(latestTaskJob(item).job_type) }}
                  {{ jobStatusLabel(latestTaskJob(item).status) }}
                </small>
                <small>{{ (item.document_ids || []).length }} 份文档 · {{ formatDate(item.updated_at) }}</small>
              </div>
              <StatusTag v-if="!latestTaskJob(item)" :status="item.status" />
              <el-tag v-else size="small" :type="jobStatusType(latestTaskJob(item).status)">
                {{ jobStatusLabel(latestTaskJob(item).status) }}
              </el-tag>
            </button>
          </div>

          <el-empty v-if="!tasks.length && !tasksLoading" description="暂无生成任务" />

          <el-form v-if="task && canReview" label-position="top" class="review-form">
            <el-form-item label="受影响产品">
              <el-select
                v-model="reviewForm.products"
                multiple
                filterable
                allow-create
                :disabled="!canReview"
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
                placeholder="留空表示不限定"
              >
                <el-option v-for="m in moduleOptions" :key="m" :value="m" :label="m" />
              </el-select>
            </el-form-item>
          </el-form>

          <div v-else-if="task" class="scope-summary">
            <div>
              <span>受影响产品</span>
              <div>
                <el-tag v-for="item in task.affected_products || []" :key="item" size="small" type="info">
                  {{ item }}
                </el-tag>
                <span v-if="!(task.affected_products || []).length">未限定</span>
              </div>
            </div>
            <div>
              <span>受影响模块</span>
              <div>
                <el-tag v-for="item in task.affected_modules || []" :key="item" size="small">
                  {{ item }}
                </el-tag>
                <span v-if="!(task.affected_modules || []).length">未限定</span>
              </div>
            </div>
          </div>

          <div class="generate-actions">
            <el-button v-if="canReview" @click="submitReview">确认范围</el-button>
            <el-button
              v-if="canGenerate"
              type="success"
              :icon="VideoPlay"
              :loading="generating"
              @click="startGenerate"
            >
              开始生成
            </el-button>
            <el-button
              v-if="canRetry"
              type="warning"
              :loading="retrying"
              @click="retryTask"
            >
              重试
            </el-button>
          </div>

          <el-tag v-if="isPolling" type="warning" class="polling-tag">后台处理中，每 3 秒刷新</el-tag>

          <el-alert
            v-if="canRetry && selectedJobError"
            :title="selectedJobError"
            type="error"
            :closable="false"
            class="inline-alert"
          />

          <el-collapse v-if="showWorkspaceDiagnostics" class="workspace-diagnostics">
            <el-collapse-item name="jobs">
              <template #title>
                <span class="diagnostic-title">诊断信息</span>
                <el-tag v-if="latestSelectedJob" size="small" :type="jobStatusType(latestSelectedJob.status)">
                  {{ jobStatusLabel(latestSelectedJob.status) }}
                </el-tag>
              </template>

              <div v-if="latestSelectedJob" class="job-summary">
                <div class="job-summary-main">
                  <strong>{{ jobTypeLabel(latestSelectedJob.job_type) }}</strong>
                  <el-tag size="small" :type="jobStatusType(latestSelectedJob.status)">
                    {{ jobStatusLabel(latestSelectedJob.status) }}
                  </el-tag>
                  <span>retry {{ latestSelectedJob.retry_count }}/{{ latestSelectedJob.max_retries }}</span>
                  <span v-if="latestSelectedJob.status === 'retrying'">
                    next {{ formatDate(latestSelectedJob.run_after) }}
                  </span>
                </div>
                <p v-if="selectedJobError" class="job-error">{{ selectedJobError }}</p>
              </div>

              <div v-if="workspaceTimeline.length" class="mini-timeline">
                <div
                  v-for="item in workspaceTimeline"
                  :key="item.key"
                  class="mini-timeline-item"
                  :class="item.status"
                >
                  <span class="mini-dot" />
                  <div>
                    <strong>{{ item.label }}</strong>
                    <small>
                      {{ jobStatusLabel(item.status) }}
                      <span v-if="item.retry"> · retry {{ item.retry }}</span>
                      <span> · {{ formatDate(item.time) }}</span>
                    </small>
                  </div>
                </div>
              </div>
            </el-collapse-item>
          </el-collapse>
        </section>
      </div>

      <section class="panel result-panel">
        <header class="panel-head">
          <div class="result-heading">
            <span class="step-index">3</span>
            <h2>测试用例输出</h2>
            <el-tag type="info">{{ caseSectionCount }} 个类别</el-tag>
            <el-tag type="success">{{ totalCaseCount }} 条用例</el-tag>
          </div>
          <div class="result-tools">
            <el-button type="primary" :icon="View" :disabled="!selectedTaskId" @click="openTaskDetail">
              审核用例
            </el-button>
            <el-button :icon="Download" :disabled="!cases.length" @click="exportCases">
              导出 JSON
            </el-button>
          </div>
        </header>

        <div v-if="task" class="result-source">
          <div class="result-source-head">
            <strong>任务输入</strong>
            <span>任务 #{{ task.id }} · {{ formatDate(task.updated_at) }}</span>
          </div>
          <div class="result-source-group">
            <span class="result-source-label">参考文档</span>
            <div class="result-source-tags">
              <el-tag v-for="doc in taskDocuments" :key="doc.id" size="small" type="info">
                {{ doc.name }}
              </el-tag>
            </div>
          </div>
          <el-alert
            v-if="!documentSelectionMatchesTask"
            title="当前勾选文档与该任务输入不同；这里仍展示任务原始结果。创建新任务后才会按新选择生成。"
            type="warning"
            :closable="false"
            show-icon
          />
        </div>

        <el-empty v-if="!cases.length && !casesLoading" :description="emptyCaseDescription" />

        <el-collapse v-else class="case-collapse">
          <el-collapse-item
            v-for="section in cases"
            :key="section.id"
            :name="section.id"
          >
            <template #title>
              <div class="section-title">
                <span>{{ section.section }}</span>
                <el-tag size="small" type="info">{{ section.cases?.length || 0 }} 条</el-tag>
                <StatusTag :status="section.status" />
              </div>
            </template>

            <el-table :data="section.cases || []" stripe size="small">
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
                        <el-table-column prop="content" label="操作" min-width="240" />
                        <el-table-column prop="expected" label="预期" min-width="240" />
                      </el-table>
                    </div>
                  </div>
                </template>
              </el-table-column>
              <el-table-column prop="title" label="标题" min-width="260" show-overflow-tooltip />
              <el-table-column label="优先级" width="100">
                <template #default="{ row }">{{ priorityLabel(row.priority_id) }}</template>
              </el-table-column>
              <el-table-column label="步骤数" width="90">
                <template #default="{ row }">{{ (row.custom_steps_separated || []).length }}</template>
              </el-table-column>
            </el-table>
          </el-collapse-item>
        </el-collapse>
      </section>
    </div>

    <el-drawer
      v-model="previewVisible"
      :title="previewDocument?.name || '参考文档预览'"
      size="88%"
      destroy-on-close
      @closed="previewDocument = null"
    >
      <div v-if="previewDocument" class="preview-meta">
        <el-tag size="small" type="info">{{ previewDocument.type }}</el-tag>
        <StatusTag :status="previewDocument.status" />
        <span class="preview-time">更新于 {{ formatDate(previewDocument.updated_at) }}</span>
      </div>
      <MarkdownPreview :content="previewDocument?.content || ''" />
    </el-drawer>

    <el-dialog
      v-model="createProjectDialog"
      title="新建项目"
      width="520px"
      :close-on-click-modal="false"
    >
      <el-form label-width="80px">
        <el-form-item label="名称">
          <el-input v-model="projectForm.name" maxlength="120" show-word-limit />
        </el-form-item>
        <el-form-item label="描述">
          <el-input
            v-model="projectForm.description"
            type="textarea"
            :rows="4"
            maxlength="500"
            show-word-limit
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createProjectDialog = false">取消</el-button>
        <el-button type="primary" :loading="projectCreating" @click="submitCreateProject">
          创建
        </el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="uploadDialog"
      title="上传参考文档"
      width="520px"
      :close-on-click-modal="false"
    >
      <el-form label-width="80px">
        <el-form-item label="文件">
          <el-upload
            :auto-upload="false"
            :show-file-list="false"
            :on-change="onFileChange"
            accept=".md,.markdown,.txt"
          >
            <el-button>选择 markdown 文件</el-button>
          </el-upload>
          <span v-if="uploadForm.file" class="file-name">{{ uploadForm.file.name }}</span>
        </el-form-item>
        <el-form-item label="名称">
          <el-input v-model="uploadForm.name" maxlength="120" show-word-limit />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="uploadDialog = false">取消</el-button>
        <el-button type="primary" :loading="uploading" @click="submitUpload">上传</el-button>
      </template>
    </el-dialog>
  </section>
</template>

<style scoped>
.generate-workspace {
  display: flex;
  flex-direction: column;
  gap: 16px;
  max-width: 1500px;
  margin: 0 auto;
}
.workspace-head,
.context-strip,
.panel {
  background: #fff;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.04);
}
.workspace-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 20px 24px;
}
.workspace-head h1 {
  margin: 0;
  font-size: 24px;
  font-weight: 650;
  color: #111827;
}
.head-actions,
.result-tools,
.generate-actions,
.section-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.head-actions :deep(.el-button + .el-button),
.result-tools :deep(.el-button + .el-button),
.generate-actions :deep(.el-button + .el-button),
.section-actions :deep(.el-button + .el-button) {
  margin-left: 0;
}
.result-source {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 0 0 14px;
  margin-bottom: 14px;
  border-bottom: 1px solid #e5e7eb;
}
.result-source-head,
.result-source-group,
.result-source-tags {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.result-source-head {
  justify-content: space-between;
  color: #64748b;
  font-size: 12px;
}
.result-source-head strong {
  color: #111827;
  font-size: 13px;
}
.result-source-label {
  width: 128px;
  flex: 0 0 128px;
  color: #475569;
  font-size: 12px;
  font-weight: 600;
}
.result-source-tags {
  flex: 1;
  min-width: 0;
}
.context-strip {
  display: grid;
  grid-template-columns: minmax(280px, 360px) minmax(0, 1fr);
  align-items: center;
  column-gap: 20px;
  padding: 12px 16px;
}
.project-select {
  display: flex;
  align-items: center;
  gap: 10px;
  color: #475569;
  font-weight: 500;
}
.project-control {
  flex: 1;
  min-width: 0;
}
.flow-strip {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 0;
  border-left: 1px solid #eef2f7;
  padding-left: 20px;
}
.flow-step {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
  padding: 8px 12px;
  border-right: 1px solid #eef2f7;
}
.flow-step:last-child {
  border-right: 0;
}
.flow-step strong,
.flow-step span {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.flow-step strong {
  color: #111827;
  font-size: 13px;
  font-weight: 700;
}
.flow-step span {
  margin-top: 2px;
  color: #64748b;
  font-size: 12px;
}
.flow-dot {
  flex: 0 0 auto;
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: #cbd5e1;
}
.flow-step.active .flow-dot {
  background: #f59e0b;
  box-shadow: 0 0 0 4px #fef3c7;
}
.flow-step.done .flow-dot {
  background: #10b981;
  box-shadow: 0 0 0 4px #d1fae5;
}
.workspace-grid {
  display: grid;
  grid-template-columns: minmax(300px, 360px) minmax(0, 1fr);
  grid-template-areas: "side result";
  gap: 16px;
  align-items: start;
}
.workspace-side {
  grid-area: side;
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 16px;
}
.panel {
  padding: 16px;
  min-width: 0;
}
.result-panel {
  grid-area: result;
  min-height: 640px;
}
.panel-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 14px;
}
.panel-head > div:first-child {
  display: flex;
  align-items: center;
  gap: 10px;
}
.panel-head h2 {
  margin: 0;
  font-size: 17px;
  font-weight: 650;
  color: #111827;
}
.result-heading {
  min-width: 0;
  flex-wrap: wrap;
}
.result-heading h2 {
  white-space: nowrap;
}
.result-tools {
  flex: 0 0 auto;
  flex-wrap: nowrap;
}
.step-index {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border-radius: 50%;
  background: #e8f0ff;
  color: #2563eb;
  font-size: 13px;
  font-weight: 700;
}
.inline-alert {
  margin-top: 12px;
  margin-bottom: 12px;
}
.doc-list,
.task-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-height: 260px;
  overflow: auto;
}
.doc-row,
.task-row {
  display: grid;
  align-items: center;
  gap: 10px;
  width: 100%;
  border: 1px solid #eef2f7;
  border-radius: 8px;
  background: #fff;
  padding: 10px 12px;
  box-sizing: border-box;
}
.doc-row {
  grid-template-columns: auto minmax(0, 1fr) auto;
  font-size: 14px;
  line-height: 1.4;
}
.doc-row.disabled {
  background: #f8fafc;
}
.doc-check {
  margin-right: 0;
}
.doc-check :deep(.el-checkbox__label) {
  display: none;
}
.doc-main {
  display: flex;
  flex: 1;
  min-width: 0;
  flex-direction: column;
  gap: 3px;
}
.doc-preview {
  display: block;
  width: 100%;
  padding: 0;
  border: 0;
  background: transparent;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: #2563eb;
  font-size: 14px;
  font-weight: 500;
  line-height: 20px;
  text-align: left;
  cursor: pointer;
}
.doc-preview:hover:not(:disabled) {
  text-decoration: underline;
}
.doc-preview:disabled {
  color: #94a3b8;
  cursor: not-allowed;
}
.doc-main small {
  color: #94a3b8;
  font-size: 12px;
  line-height: 16px;
}
.preview-meta {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-bottom: 4px;
}
.preview-time {
  color: #909399;
}
.task-row {
  display: flex;
  justify-content: space-between;
  gap: 10px;
  color: #111827;
  cursor: pointer;
}
.task-row-main {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 2px;
  min-width: 0;
}
.task-row-main small {
  color: #64748b;
  font-size: 12px;
}
.task-row.active {
  border-color: #93c5fd;
  background: #f0f7ff;
}
.job-summary {
  margin-top: 12px;
  padding: 10px 12px;
  border: 1px solid #eef2f7;
  border-radius: 8px;
  background: #f8fafc;
}
.job-summary-main {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  color: #475569;
  font-size: 13px;
}
.job-summary-main strong {
  color: #111827;
}
.job-error {
  margin: 6px 0 0;
  color: #f56c6c;
  font-size: 12px;
  word-break: break-word;
}
.scope-summary {
  display: grid;
  gap: 10px;
  margin-top: 10px;
}
.scope-summary > div {
  display: grid;
  grid-template-columns: 88px minmax(0, 1fr);
  align-items: start;
  gap: 10px;
  color: #64748b;
  font-size: 13px;
}
.scope-summary > div > div {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
  color: #111827;
}
.mini-timeline {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-top: 12px;
  padding-left: 4px;
}
.mini-timeline-item {
  display: grid;
  grid-template-columns: 14px minmax(0, 1fr);
  gap: 8px;
  align-items: start;
  color: #475569;
}
.mini-dot {
  width: 9px;
  height: 9px;
  margin-top: 5px;
  border-radius: 50%;
  background: #94a3b8;
}
.mini-timeline-item.running .mini-dot,
.mini-timeline-item.retrying .mini-dot {
  background: #e6a23c;
}
.mini-timeline-item.failed .mini-dot {
  background: #f56c6c;
}
.mini-timeline-item.succeeded .mini-dot {
  background: #67c23a;
}
.mini-timeline-item strong {
  display: block;
  color: #111827;
  font-size: 13px;
}
.mini-timeline-item small {
  color: #64748b;
  font-size: 12px;
}
.panel-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-top: 14px;
  color: #64748b;
  font-size: 13px;
}
.review-form {
  margin-top: 16px;
}
.review-form :deep(.el-select) {
  width: 100%;
}
.polling-tag {
  margin-top: 12px;
}
.workspace-diagnostics {
  margin-top: 12px;
  border-top: 1px solid #eef2f7;
  border-bottom: 0;
}
.workspace-diagnostics :deep(.el-collapse-item__header) {
  gap: 8px;
  min-height: 42px;
  line-height: 1.2;
}
.workspace-diagnostics :deep(.el-collapse-item__content) {
  padding-bottom: 4px;
}
.diagnostic-title {
  color: #111827;
  font-size: 13px;
  font-weight: 650;
}
.case-collapse {
  border-top: 1px solid #eef2f7;
}
.result-panel :deep(.el-collapse-item__content) {
  padding-bottom: 12px;
}
.section-title {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  font-weight: 600;
}
.section-title span:first-child {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
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
  color: #64748b;
  font-size: 13px;
  font-weight: 600;
}
.field-body {
  color: #111827;
  line-height: 1.6;
  word-break: break-word;
}
.steps-table {
  width: 100%;
}
.file-name {
  margin-left: 12px;
  color: #475569;
}
@media (max-width: 1180px) {
  .workspace-grid {
    grid-template-areas:
      "side"
      "result";
    grid-template-columns: 1fr;
  }
  .result-panel {
    min-height: 520px;
  }
  .document-panel {
    order: 1;
  }
  .task-panel {
    order: 2;
  }
}
@media (max-width: 760px) {
  .workspace-head,
  .context-strip,
  .flow-strip,
  .project-select {
    align-items: stretch;
    flex-direction: column;
  }
  .context-strip {
    display: flex;
    gap: 12px;
  }
  .flow-strip {
    display: flex;
    border-top: 1px solid #eef2f7;
    border-left: 0;
    padding-left: 0;
  }
  .flow-step {
    border-right: 0;
    border-bottom: 1px solid #eef2f7;
  }
  .flow-step:last-child {
    border-bottom: 0;
  }
  .project-control {
    width: 100%;
  }
  .head-actions,
  .result-tools,
  .generate-actions {
    align-items: stretch;
  }
}
</style>
