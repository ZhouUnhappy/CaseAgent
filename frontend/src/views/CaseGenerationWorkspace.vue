<script setup>
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { storeToRefs } from 'pinia'
import { ElMessageBox } from 'element-plus'
import { Check, Download, Plus, Refresh, Upload, VideoPlay, View } from '@element-plus/icons-vue'
import StatusTag from '../components/StatusTag.vue'
import { useProjectsStore } from '../stores/projects'
import { useDocumentsStore } from '../stores/documents'
import { useTasksStore } from '../stores/tasks'
import { useTestCasesStore } from '../stores/testcases'
import { useKnowledgeStore } from '../stores/knowledge'
import { notifySuccess } from '../utils/error'

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

const createProjectDialog = ref(false)
const projectForm = reactive({ name: '', description: '' })

const uploadDialog = ref(false)
const uploadForm = reactive({ name: '', file: null })

const reviewForm = reactive({ products: [], modules: [] })

const selectedProject = computed(() =>
  projects.value.find((project) => project.id === selectedProjectId.value) || null,
)

const task = computed(() => {
  if (loadedTask.value?.id === selectedTaskId.value) return loadedTask.value
  return tasks.value.find((item) => item.id === selectedTaskId.value) || null
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
  return task.value.status
})
const selectedDocumentCount = computed(() => selectedDocumentIds.value.length)
const primaryAction = computed(() => {
  if (!selectedProjectId.value) {
    return {
      label: '新建项目',
      icon: Plus,
      type: 'primary',
      disabled: false,
      loading: projectCreating.value,
      handler: openCreateProject,
    }
  }
  if (!completedDocuments.value.length) {
    return {
      label: '上传需求文档',
      icon: Upload,
      type: 'primary',
      disabled: false,
      loading: uploading.value,
      handler: openUpload,
    }
  }
  if (!task.value) {
    return {
      label: '创建生成任务',
      icon: Plus,
      type: 'primary',
      disabled: selectedDocumentIds.value.length === 0,
      loading: taskCreating.value,
      handler: createTaskFromDocuments,
    }
  }
  if (task.value?.status === 'awaiting_review') {
    return {
      label: '确认影响范围',
      icon: Check,
      type: 'primary',
      disabled: false,
      loading: false,
      handler: submitReview,
    }
  }
  if (canGenerate.value) {
    return {
      label: '开始生成',
      icon: VideoPlay,
      type: 'success',
      disabled: false,
      loading: generating.value,
      handler: startGenerate,
    }
  }
  if (canRetry.value) {
    return {
      label: '重试任务',
      icon: Refresh,
      type: 'warning',
      disabled: false,
      loading: retrying.value,
      handler: retryTask,
    }
  }
  if (cases.value.length) {
    return {
      label: '导出 JSON',
      icon: Download,
      type: 'primary',
      disabled: false,
      loading: false,
      handler: exportCases,
    }
  }
  return {
    label: isPolling.value ? '后台处理中' : '等待下一步',
    icon: Refresh,
    type: 'primary',
    disabled: true,
    loading: Boolean(isPolling.value),
    handler: () => {},
  }
})
const flowSteps = computed(() => {
  const status = task.value?.status || ''
  const hasDocuments = completedDocuments.value.length > 0
  const hasReviewedScope = ['ready_to_generate', 'generating', 'completed'].includes(status)
  const hasGenerationStarted = ['generating', 'completed'].includes(status)
  return [
    {
      label: '文档',
      value: `${completedDocuments.value.length} 可用`,
      state: hasDocuments ? 'done' : selectedProjectId.value ? 'active' : 'waiting',
    },
    {
      label: '范围',
      value: task.value ? status : '未创建任务',
      state: hasReviewedScope ? 'done' : task.value ? 'active' : 'waiting',
    },
    {
      label: '生成',
      value: totalCaseCount.value ? `${totalCaseCount.value} 条` : status || '等待',
      state: totalCaseCount.value ? 'done' : hasGenerationStarted ? 'active' : 'waiting',
    },
    {
      label: '输出',
      value: caseSectionCount.value ? `${caseSectionCount.value} sections` : '等待',
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
  casesStore.clear()

  if (!projectID) return
  await Promise.all([
    documentsStore.fetch(projectID).catch(() => {}),
    tasksStore.fetch(projectID).catch(() => {}),
  ])

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
  await Promise.all([
    documentsStore.fetch(selectedProjectId.value).catch(() => {}),
    tasksStore.fetch(selectedProjectId.value).catch(() => {}),
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
    ElMessageBox.alert('请选择至少一份已完成处理的需求文档')
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
  try {
    const loaded = await tasksStore.load(row.id)
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

async function submitSection(section) {
  try {
    await ElMessageBox.confirm(
      `提交 section「${section.section}」共 ${section.cases?.length || 0} 条用例？`,
      '确认提交',
      { type: 'info' },
    )
  } catch {
    return
  }
  try {
    await casesStore.submit(selectedTaskId.value, section.id)
    notifySuccess('已提交')
  } catch {
    /* 错误已弹窗 */
  }
}

function ensurePolling() {
  if (polling.value || !selectedTaskId.value) return
  polling.value = setInterval(() => {
    tasksStore.load(selectedTaskId.value).catch(() => {})
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

function formatDate(value) {
  return value ? new Date(value).toLocaleString() : '-'
}

function priorityLabel(id) {
  return { 1: 'Low', 2: 'Medium', 3: 'High', 4: 'Critical' }[id] || `P${id}`
}
</script>

<template>
  <section class="generate-workspace">
    <div class="workspace-head">
      <div class="head-copy">
        <h1>生成测试用例</h1>
        <div class="head-metrics">
          <span>{{ selectedDocumentCount }} 份已选文档</span>
          <span>任务：{{ taskStatusText }}</span>
          <span>{{ totalCaseCount }} 条用例</span>
        </div>
      </div>
      <div class="head-actions">
        <el-button :icon="Refresh" @click="refreshProjectData" :disabled="!selectedProjectId">
          刷新
        </el-button>
        <el-button
          :type="primaryAction.type"
          :icon="primaryAction.icon"
          :disabled="primaryAction.disabled"
          :loading="primaryAction.loading"
          @click="primaryAction.handler"
        >
          {{ primaryAction.label }}
        </el-button>
        <el-button v-if="projects.length" :icon="Plus" @click="openCreateProject">新建项目</el-button>
      </div>
    </div>

    <div class="project-strip">
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
      <div class="project-summary" v-if="selectedProject">
        <strong>{{ selectedProject.name }}</strong>
        <span>{{ selectedProject.description || '未填写描述' }}</span>
      </div>
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

    <el-empty
      v-if="!projects.length && !projectsLoading"
      description="暂无项目，先新建一个项目"
    >
      <el-button type="primary" :icon="Plus" @click="openCreateProject">新建项目</el-button>
    </el-empty>

    <div v-else class="workspace-grid">
      <section class="panel document-panel">
        <header class="panel-head">
          <div>
            <span class="step-index">1</span>
            <h2>需求文档</h2>
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
              <span>{{ doc.name }}</span>
              <small>#{{ doc.id }} · {{ formatDate(doc.updated_at) }}</small>
            </div>
            <StatusTag class="doc-status" :status="doc.status" />
          </div>
        </el-checkbox-group>

        <el-empty v-if="!documents.length && !documentsLoading" description="暂无需求文档" />

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
            <span>任务 #{{ item.id }}</span>
            <StatusTag :status="item.status" />
          </button>
        </div>

        <el-empty v-if="!tasks.length && !tasksLoading" description="暂无生成任务" />

        <el-form v-if="task" label-position="top" class="review-form">
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
          <el-button :icon="View" :disabled="!selectedTaskId" @click="openTaskDetail">
            任务详情
          </el-button>
        </div>

        <el-tag v-if="isPolling" type="warning" class="polling-tag">后台处理中，每 3 秒刷新</el-tag>
      </section>

      <section class="panel result-panel">
        <header class="panel-head">
          <div>
            <span class="step-index">3</span>
            <h2>测试用例输出</h2>
          </div>
          <div class="result-tools">
            <el-tag type="info">{{ caseSectionCount }} sections</el-tag>
            <el-tag type="success">{{ totalCaseCount }} cases</el-tag>
            <el-button :icon="Refresh" :disabled="!selectedTaskId" :loading="casesLoading" @click="refreshCases">
              刷新
            </el-button>
            <el-button type="primary" :icon="Download" :disabled="!cases.length" @click="exportCases">
              导出 JSON
            </el-button>
          </div>
        </header>

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
                <el-tag size="small" type="info">{{ section.cases?.length || 0 }} cases</el-tag>
                <StatusTag :status="section.status" />
              </div>
            </template>

            <div class="section-actions">
              <el-button
                size="small"
                type="primary"
                :disabled="section.status === 'submitted' || section.status === 'approved'"
                @click="submitSection(section)"
              >
                提交 section
              </el-button>
            </div>

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
      title="上传需求文档"
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
.project-strip,
.flow-strip,
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
.head-copy {
  min-width: 0;
}
.workspace-head h1 {
  margin: 0;
  font-size: 24px;
  font-weight: 650;
  color: #111827;
}
.head-metrics {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-top: 10px;
}
.head-metrics span {
  display: inline-flex;
  align-items: center;
  min-height: 26px;
  padding: 0 10px;
  border-radius: 8px;
  background: #f1f5f9;
  color: #475569;
  font-size: 12px;
  font-weight: 600;
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
.project-strip {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
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
  width: 280px;
}
.project-summary {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
  color: #64748b;
}
.project-summary strong {
  color: #111827;
}
.flow-strip {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 0;
  padding: 12px 16px;
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
  grid-template-areas:
    "task result"
    "documents result";
  gap: 16px;
  align-items: stretch;
}
.panel {
  padding: 16px;
  min-width: 0;
}
.document-panel {
  grid-area: documents;
}
.task-panel {
  grid-area: task;
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
.doc-main span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: #111827;
  font-size: 14px;
  font-weight: 500;
  line-height: 20px;
}
.doc-main small {
  color: #94a3b8;
  font-size: 12px;
  line-height: 16px;
}
.task-row {
  display: flex;
  justify-content: space-between;
  color: #111827;
  cursor: pointer;
}
.task-row.active {
  border-color: #93c5fd;
  background: #f0f7ff;
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
      "documents"
      "task"
      "result";
    grid-template-columns: 1fr;
  }
  .result-panel {
    min-height: 520px;
  }
}
@media (max-width: 760px) {
  .workspace-head,
  .project-strip,
  .flow-strip,
  .project-summary,
  .project-select {
    align-items: stretch;
    flex-direction: column;
  }
  .flow-strip {
    display: flex;
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
