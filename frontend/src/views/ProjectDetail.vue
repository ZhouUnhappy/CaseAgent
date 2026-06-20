<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { storeToRefs } from 'pinia'
import { ElMessageBox } from 'element-plus'
import { MagicStick, View } from '@element-plus/icons-vue'
import MarkdownPreview from '../components/MarkdownPreview.vue'
import StatusTag from '../components/StatusTag.vue'
import { listJobs } from '../api/jobs'
import { getProject } from '../api/projects'
import { useDocumentsStore } from '../stores/documents'
import { useTasksStore } from '../stores/tasks'
import { notifySuccess } from '../utils/error'
import { compactJobError, jobStatusLabel, jobStatusType, jobTypeLabel, latestJob } from '../utils/jobs'

const route = useRoute()
const router = useRouter()
const projectId = computed(() => Number(route.params.id))

const project = ref(null)
const documents = useDocumentsStore()
const tasks = useTasksStore()
const { items: docItems, loading: docLoading, uploading } = storeToRefs(documents)
const { items: taskItems, loading: taskLoading, creating: taskCreating } = storeToRefs(tasks)

const uploadDialog = ref(false)
const uploadForm = reactive({ name: '', file: null })
const uploadFormRef = ref(null)
const previewVisible = ref(false)
const previewDocument = ref(null)
const documentJobs = ref({})
const taskJobs = ref({})

const taskDialog = ref(false)
const taskForm = reactive({ documentIds: [] })
const taskFormRef = ref(null)

onMounted(() => {
  refresh()
})

async function refresh() {
  try {
    project.value = await getProject(projectId.value)
  } catch {
    /* api/client.js 已弹错 */
  }
  await Promise.all([
    documents.fetch(projectId.value).catch(() => {}),
    tasks.fetch(projectId.value).catch(() => {}),
  ])
  loadDocumentJobs().catch(() => {})
  loadTaskJobs().catch(() => {})
}

function formatDate(value) {
  return value ? new Date(value).toLocaleString() : '-'
}

function openUpload() {
  uploadForm.name = ''
  uploadForm.file = null
  uploadDialog.value = true
}

function openDocumentPreview(document) {
  previewDocument.value = document
  previewVisible.value = true
}

function onFileChange(file) {
  uploadForm.file = file?.raw || null
  if (!uploadForm.name && file?.name) {
    uploadForm.name = file.name
  }
}

async function submitUpload() {
  if (!uploadForm.file) {
    ElMessageBox.alert('请选择文件')
    return
  }
  if (!uploadForm.name) {
    ElMessageBox.alert('请输入文档名称')
    return
  }
  try {
    await documents.upload(projectId.value, {
      name: uploadForm.name,
      type: 'markdown',
      source: 'upload',
      file: uploadForm.file,
    })
    notifySuccess('文档已上传，后台正在分块/向量化')
    uploadDialog.value = false
    loadDocumentJobs().catch(() => {})
  } catch {
    /* 错误已弹窗 */
  }
}

async function reprocessDocument(doc) {
  try {
    await documents.reprocess(doc.id)
    notifySuccess(`文档 ${doc.name} 重新处理已触发`)
    loadDocumentJobs().catch(() => {})
  } catch {
    /* 错误已弹窗 */
  }
}

async function removeDocument(doc) {
  try {
    await ElMessageBox.confirm(`删除文档「${doc.name}」？`, '确认', { type: 'warning' })
  } catch {
    return
  }
  try {
    await documents.remove(doc.id)
    notifySuccess('文档已删除')
    const next = { ...documentJobs.value }
    delete next[doc.id]
    documentJobs.value = next
  } catch {
    /* 错误已弹窗 */
  }
}

const completedDocOptions = computed(() =>
  docItems.value.map((d) => ({
    value: d.id,
    label: `${d.name} (#${d.id})`,
    disabled: d.status !== 'completed',
    status: d.status,
  })),
)

function openCreateTask() {
  taskForm.documentIds = []
  taskDialog.value = true
}

async function submitCreateTask() {
  if (taskForm.documentIds.length === 0) {
    ElMessageBox.alert('请至少选择一份已完成处理的文档')
    return
  }
  try {
    const created = await tasks.create(projectId.value, {
      document_ids: taskForm.documentIds,
    })
    notifySuccess(`任务 #${created.id} 已创建，正在分析影响范围`)
    taskDialog.value = false
    router.push({ name: 'task-detail', params: { id: created.id } })
  } catch {
    /* 错误已弹窗 */
  }
}

function viewTask(task) {
  if (task.status === 'completed') {
    router.push({
      name: 'generate',
      query: { project_id: projectId.value, task_id: task.id },
    })
    return
  }
  router.push({ name: 'task-detail', params: { id: task.id } })
}

function openGenerateWorkspace() {
  router.push({ name: 'generate', query: { project_id: projectId.value } })
}

async function loadDocumentJobs() {
  const entries = await Promise.all(
    docItems.value.map(async (doc) => [doc.id, latestJob(await listJobs({ document_id: doc.id }))]),
  )
  documentJobs.value = Object.fromEntries(entries.filter(([, job]) => job))
}

async function loadTaskJobs() {
  const entries = await Promise.all(
    taskItems.value.map(async (task) => [task.id, latestJob(await listJobs({ task_id: task.id }))]),
  )
  taskJobs.value = Object.fromEntries(entries.filter(([, job]) => job))
}

function latestDocumentJob(row) {
  return documentJobs.value[row.id] || null
}

function latestTaskJob(row) {
  return taskJobs.value[row.id] || null
}
</script>

<template>
  <section class="project-detail">
    <header class="page-header">
      <div class="page-heading">
        <el-breadcrumb separator="/">
          <el-breadcrumb-item :to="{ name: 'projects' }">项目</el-breadcrumb-item>
          <el-breadcrumb-item>{{ project?.name || `#${projectId}` }}</el-breadcrumb-item>
        </el-breadcrumb>
        <p v-if="project?.description" class="hint">{{ project.description }}</p>
      </div>
      <div class="page-actions">
        <el-button type="primary" :icon="MagicStick" @click="openGenerateWorkspace">
          生成用例
        </el-button>
      </div>
    </header>

    <el-card shadow="never" class="card">
      <template #header>
        <div class="card-header">
          <span>需求文档</span>
          <div class="header-actions">
            <el-button @click="documents.fetch(projectId)" :loading="docLoading">刷新</el-button>
            <el-button type="primary" @click="openUpload">上传文档</el-button>
          </div>
        </div>
      </template>
      <el-table :data="docItems" v-loading="docLoading" stripe>
        <el-table-column prop="id" label="ID" width="70" />
        <el-table-column prop="name" label="名称" min-width="220" show-overflow-tooltip />
        <el-table-column prop="type" label="类型" width="120" />
        <el-table-column label="状态" width="140">
          <template #default="{ row }"><StatusTag :status="row.status" /></template>
        </el-table-column>
        <el-table-column label="最近任务" min-width="240">
          <template #default="{ row }">
            <div v-if="latestDocumentJob(row)" class="job-cell">
              <div class="job-line">
                <span>{{ jobTypeLabel(latestDocumentJob(row).job_type) }}</span>
                <el-tag size="small" :type="jobStatusType(latestDocumentJob(row).status)">
                  {{ jobStatusLabel(latestDocumentJob(row).status) }}
                </el-tag>
                <span class="muted small">
                  {{ latestDocumentJob(row).retry_count }}/{{ latestDocumentJob(row).max_retries }}
                </span>
              </div>
              <span
                v-if="row.status === 'failed' && latestDocumentJob(row).last_error"
                class="muted danger small"
              >{{ compactJobError(latestDocumentJob(row)) }}</span>
            </div>
            <span v-else class="muted">-</span>
          </template>
        </el-table-column>
        <el-table-column label="更新时间" width="180">
          <template #default="{ row }">{{ formatDate(row.updated_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="300">
          <template #default="{ row }">
            <el-button
              size="small"
              :icon="View"
              :disabled="!row.content"
              @click="openDocumentPreview(row)"
            >预览</el-button>
            <el-button
              size="small"
              :disabled="row.status === 'processing'"
              @click="reprocessDocument(row)"
            >重新处理</el-button>
            <el-button size="small" type="danger" @click="removeDocument(row)">删除</el-button>
          </template>
        </el-table-column>
        <template #empty>暂无文档，点击右上角上传。</template>
      </el-table>
    </el-card>

    <el-card shadow="never" class="card">
      <template #header>
        <div class="card-header">
          <span>生成任务与测试用例</span>
          <div class="header-actions">
            <el-button @click="tasks.fetch(projectId)" :loading="taskLoading">刷新</el-button>
            <el-button type="primary" @click="openCreateTask">新建任务</el-button>
          </div>
        </div>
      </template>
      <el-table :data="taskItems" v-loading="taskLoading" stripe>
        <el-table-column prop="id" label="ID" width="80" />
        <el-table-column label="状态" width="160">
          <template #default="{ row }"><StatusTag :status="row.status" /></template>
        </el-table-column>
        <el-table-column label="阶段" min-width="210">
          <template #default="{ row }">
            <div v-if="latestTaskJob(row)" class="job-cell">
              <div class="job-line">
                <span>{{ jobTypeLabel(latestTaskJob(row).job_type) }}</span>
                <el-tag size="small" :type="jobStatusType(latestTaskJob(row).status)">
                  {{ jobStatusLabel(latestTaskJob(row).status) }}
                </el-tag>
              </div>
              <span
                v-if="['retrying', 'failed'].includes(latestTaskJob(row).status)"
                class="muted small"
              >
                retry {{ latestTaskJob(row).retry_count }}/{{ latestTaskJob(row).max_retries }}
              </span>
            </div>
            <span v-else class="muted">{{ row.status }}</span>
          </template>
        </el-table-column>
        <el-table-column label="影响产品" min-width="180">
          <template #default="{ row }">
            <el-tag
              v-for="p in row.affected_products || []"
              :key="p"
              size="small"
              class="chip"
              type="info"
            >{{ p }}</el-tag>
            <span v-if="!(row.affected_products || []).length" class="muted">-</span>
          </template>
        </el-table-column>
        <el-table-column label="影响模块" min-width="180">
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
        <el-table-column label="文档数" width="100">
          <template #default="{ row }">{{ (row.document_ids || []).length }}</template>
        </el-table-column>
        <el-table-column label="测试用例" width="120">
          <template #default="{ row }">
            <el-tag v-if="row.status === 'completed'" type="success" size="small">已生成</el-tag>
            <span v-else class="muted">-</span>
          </template>
        </el-table-column>
        <el-table-column label="更新时间" width="180">
          <template #default="{ row }">{{ formatDate(row.updated_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="120">
          <template #default="{ row }">
            <el-button size="small" type="primary" link @click="viewTask(row)">
              {{ row.status === 'completed' ? '查看用例' : '详情' }}
            </el-button>
          </template>
        </el-table-column>
        <template #empty>暂无任务，点击右上角新建。</template>
      </el-table>
    </el-card>

    <el-drawer
      v-model="previewVisible"
      :title="previewDocument?.name || '需求文档预览'"
      size="min(860px, 94vw)"
      destroy-on-close
      @closed="previewDocument = null"
    >
      <div v-if="previewDocument" class="preview-meta">
        <el-tag size="small" type="info">{{ previewDocument.type }}</el-tag>
        <StatusTag :status="previewDocument.status" />
        <span class="muted">更新于 {{ formatDate(previewDocument.updated_at) }}</span>
      </div>
      <MarkdownPreview :content="previewDocument?.content || ''" />
    </el-drawer>

    <el-dialog
      v-model="uploadDialog"
      title="上传需求文档"
      width="520px"
      :close-on-click-modal="false"
    >
      <el-form ref="uploadFormRef" :model="uploadForm" label-width="80px">
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

    <el-dialog
      v-model="taskDialog"
      title="新建生成任务"
      width="520px"
      :close-on-click-modal="false"
    >
      <el-form ref="taskFormRef" :model="taskForm" label-width="80px">
        <el-form-item label="文档">
          <el-select
            v-model="taskForm.documentIds"
            multiple
            collapse-tags
            collapse-tags-tooltip
            placeholder="选择已完成处理的文档"
            style="width: 100%"
          >
            <el-option
              v-for="opt in completedDocOptions"
              :key="opt.value"
              :value="opt.value"
              :label="opt.label"
              :disabled="opt.disabled"
            >
              <span style="float:left">{{ opt.label }}</span>
              <span style="float:right; color: #909399; font-size: 12px">{{ opt.status }}</span>
            </el-option>
          </el-select>
        </el-form-item>
        <p class="muted small">仅 status = completed 的文档可被选中；analyze 阶段会自动推断影响范围。</p>
      </el-form>
      <template #footer>
        <el-button @click="taskDialog = false">取消</el-button>
        <el-button type="primary" :loading="taskCreating" @click="submitCreateTask">创建</el-button>
      </template>
    </el-dialog>
  </section>
</template>

<style scoped>
.project-detail {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.page-header {
  background: #fff;
  border-radius: 8px;
  padding: 16px 24px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}
.page-heading {
  min-width: 0;
}
.hint {
  margin: 8px 0 0;
  color: #909399;
}
.page-actions {
  flex: 0 0 auto;
}
.card {
  border-radius: 8px;
}
.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.header-actions {
  display: flex;
  gap: 8px;
}
.chip {
  margin-right: 4px;
}
.job-cell {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}
.job-line {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}
.job-cell .small {
  margin: 0;
}
.muted {
  color: #909399;
}
.danger {
  color: #f56c6c;
  word-break: break-word;
}
.small {
  font-size: 12px;
  margin: 8px 0 0;
}
.file-name {
  margin-left: 12px;
  color: #606266;
}
.preview-meta {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-bottom: 4px;
}
</style>
