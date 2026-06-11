<script setup>
import { onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { storeToRefs } from 'pinia'
import { ElMessageBox } from 'element-plus'
import StatusTag from '../components/StatusTag.vue'
import { listJobs } from '../api/jobs'
import { listKnowledgeImpactedTasks } from '../api/knowledge'
import { updateKnowledgeSuggestion } from '../api/knowledgeSuggestions'
import { useKnowledgeStore } from '../stores/knowledge'
import { readAndClearSuggestionDraft } from '../utils/knowledgeSuggestionDraft'
import { notifySuccess } from '../utils/error'
import { compactJobError, jobStatusLabel, jobStatusType, jobTypeLabel, latestJob } from '../utils/jobs'

const route = useRoute()
const router = useRouter()
const store = useKnowledgeStore()
const { items, loading, saving, typeFilter, sourceFilter, expiredFilter, duplicateFilter } = storeToRefs(store)

const dialogVisible = ref(false)
const editing = ref(null)
const form = reactive({
  type: 'product',
  name: '',
  content: '',
  metadata: '',
  source: 'manual',
  expires_at: '',
  duplicate_of_id: null,
})
const formRef = ref(null)
const knowledgeJobs = ref({})
const impactDialogVisible = ref(false)
const impactLoading = ref(false)
const impactRows = ref([])
const impactKnowledge = ref(null)

const rules = {
  type: [{ required: true, message: '请选择类型', trigger: 'change' }],
  name: [{ required: true, message: '请输入名称', trigger: 'blur' }],
  content: [{ required: true, message: '请输入内容', trigger: 'blur' }],
}

onMounted(() => {
  refreshKnowledge().catch(() => {})

  // Knowledge-suggestion adopt flow: prefill the create dialog from the
  // query string so the operator only needs to fill in content.
  const {
    create_type: createType,
    create_name: createName,
    type,
    name,
  } = route.query
  const targetType = createType || type
  const targetName = createName || name
  if (targetType && targetName) {
    const suggestionID = Number(route.query.from_suggestion_id)
    const draftContent = readAndClearSuggestionDraft(suggestionID)
    editing.value = null
    Object.assign(form, {
      type: targetType === 'product' ? 'product' : 'module',
      name: String(targetName),
      content: draftContent,
      metadata: '',
      source: 'suggestion',
      expires_at: '',
      duplicate_of_id: null,
    })
    dialogVisible.value = true
  }
})

async function refreshKnowledge() {
  await store.fetch()
  loadKnowledgeJobs().catch(() => {})
}

async function changeTypeFilter(value) {
  await store.setTypeFilter(value)
  loadKnowledgeJobs().catch(() => {})
}

async function changeFilters(patch) {
  await store.setFilters(patch)
  loadKnowledgeJobs().catch(() => {})
}

function formatDate(value) {
  return value ? new Date(value).toLocaleString() : '-'
}

function rowSource(row) {
  return row.source || 'manual'
}

function isExpired(row) {
  return row.expires_at ? new Date(row.expires_at).getTime() <= Date.now() : false
}

function expiryLabel(row) {
  if (!row.expires_at) return '长期有效'
  return isExpired(row) ? `已过期 ${formatDate(row.expires_at)}` : formatDate(row.expires_at)
}

function expiryTagType(row) {
  if (!row.expires_at) return 'info'
  return isExpired(row) ? 'danger' : 'success'
}

function duplicateLabel(row) {
  return row.duplicate_of_id ? `#${row.duplicate_of_id}` : '-'
}

function openCreate() {
  editing.value = null
  Object.assign(form, {
    type: 'product',
    name: '',
    content: '',
    metadata: '',
    source: 'manual',
    expires_at: '',
    duplicate_of_id: null,
  })
  dialogVisible.value = true
}

function openEdit(row) {
  editing.value = row
  form.type = row.type
  form.name = row.name
  form.content = row.content
  form.metadata = row.metadata ? JSON.stringify(row.metadata, null, 2) : ''
  form.source = rowSource(row)
  form.expires_at = row.expires_at || ''
  form.duplicate_of_id = row.duplicate_of_id || null
  dialogVisible.value = true
}

function parseMetadata() {
  if (!form.metadata.trim()) return null
  try {
    const parsed = JSON.parse(form.metadata)
    if (typeof parsed !== 'object' || Array.isArray(parsed) || parsed === null) {
      throw new Error('必须是 JSON 对象')
    }
    return parsed
  } catch (err) {
    throw new Error(`metadata 解析失败：${err.message}`)
  }
}

function normalizeDuplicateID(value) {
  if (value === null || value === undefined || value === '') return null
  const parsed = Number(value)
  if (!Number.isInteger(parsed) || parsed <= 0) {
    throw new Error('重复指向必须是正整数知识 ID，或留空')
  }
  return parsed
}

async function submit() {
  if (!formRef.value) return
  const ok = await formRef.value.validate().catch(() => false)
  if (!ok) return
  let metadata = null
  let duplicateID = null
  try {
    metadata = parseMetadata()
    duplicateID = normalizeDuplicateID(form.duplicate_of_id)
  } catch (err) {
    ElMessageBox.alert(err.message)
    return
  }
  const payload = {
    type: form.type,
    name: form.name,
    content: form.content,
    metadata,
    source: form.source || 'manual',
    expires_at: form.expires_at || null,
    duplicate_of_id: duplicateID,
  }
  try {
    let saved
    if (editing.value) {
      saved = await store.update(editing.value.id, {
        name: payload.name,
        content: payload.content,
        metadata: payload.metadata,
        source: payload.source,
        expires_at: payload.expires_at,
        clear_expires_at: !payload.expires_at,
        duplicate_of_id: payload.duplicate_of_id,
        clear_duplicate: !payload.duplicate_of_id,
      })
      notifySuccess('知识条目已更新')
    } else {
      saved = await store.create(payload)
      notifySuccess('知识条目已创建，后台正在向量化')
    }
    loadKnowledgeJobs().catch(() => {})
    try {
      await markSourceSuggestionAdopted(saved.id)
    } catch {
      /* 知识条目已经保存；关联失败由 api/client.js 弹错，用户可回建议列表重试。 */
    }
    dialogVisible.value = false
  } catch {
    /* api/client.js 已弹错 */
  }
}

async function showImpactedTasks(row) {
  impactKnowledge.value = row
  impactRows.value = []
  impactDialogVisible.value = true
  impactLoading.value = true
  try {
    impactRows.value = await listKnowledgeImpactedTasks(row.id)
  } catch {
    /* api/client.js 已弹错 */
  } finally {
    impactLoading.value = false
  }
}

function openTask(row) {
  router.push({ name: 'task-detail', params: { id: row.task_id } }).catch(() => {})
}

async function markSourceSuggestionAdopted(knowledgeID) {
  const suggestionID = Number(route.query.from_suggestion_id)
  if (!suggestionID || !knowledgeID) return

  await updateKnowledgeSuggestion(suggestionID, 'adopted', knowledgeID)
  await router.replace({ name: 'knowledge', query: {} })
  notifySuccess('知识建议已关联到新知识条目')
}

async function reprocess(row) {
  try {
    await store.reprocess(row.id)
    notifySuccess(`知识 ${row.name} 重新向量化已触发`)
    loadKnowledgeJobs().catch(() => {})
  } catch {
    /* 错误已弹窗 */
  }
}

async function remove(row) {
  try {
    await ElMessageBox.confirm(
      `确定要删除知识条目「${row.name}」吗？此操作不可恢复。`,
      '删除知识条目',
      {
        type: 'warning',
        confirmButtonText: '删除',
        cancelButtonText: '取消',
        confirmButtonClass: 'el-button--danger',
        draggable: false,
      },
    )
  } catch {
    return
  }
  try {
    await store.remove(row.id)
    notifySuccess('已删除')
    const next = { ...knowledgeJobs.value }
    delete next[row.id]
    knowledgeJobs.value = next
  } catch {
    /* 错误已弹窗 */
  }
}

async function loadKnowledgeJobs() {
  const entries = await Promise.all(
    items.value.map(async (row) => [row.id, latestJob(await listJobs({ knowledge_id: row.id }))]),
  )
  knowledgeJobs.value = Object.fromEntries(entries.filter(([, job]) => job))
}

function latestKnowledgeJob(row) {
  return knowledgeJobs.value[row.id] || null
}
</script>

<template>
  <section class="knowledge">
    <header class="bar">
      <div>
        <h2>知识库</h2>
        <p class="hint">维护 product / module 两类架构知识，供 analyze 阶段推断影响范围。</p>
      </div>
      <div class="actions">
        <el-radio-group
          :model-value="typeFilter"
          @change="(v) => changeTypeFilter(v).catch(() => {})"
        >
          <el-radio-button value="">全部</el-radio-button>
          <el-radio-button value="product">product</el-radio-button>
          <el-radio-button value="module">module</el-radio-button>
        </el-radio-group>
        <el-input
          v-model="sourceFilter"
          class="source-filter"
          clearable
          placeholder="来源"
          @change="(v) => changeFilters({ sourceFilter: v }).catch(() => {})"
          @clear="() => changeFilters({ sourceFilter: '' }).catch(() => {})"
        />
        <el-select
          v-model="expiredFilter"
          class="state-filter"
          placeholder="有效期"
          @change="(v) => changeFilters({ expiredFilter: v }).catch(() => {})"
        >
          <el-option label="全部有效期" value="" />
          <el-option label="未过期" value="false" />
          <el-option label="已过期" value="true" />
        </el-select>
        <el-select
          v-model="duplicateFilter"
          class="state-filter"
          placeholder="重复"
          @change="(v) => changeFilters({ duplicateFilter: v }).catch(() => {})"
        >
          <el-option label="全部条目" value="" />
          <el-option label="原始条目" value="false" />
          <el-option label="重复条目" value="true" />
        </el-select>
        <el-button @click="refreshKnowledge" :loading="loading">刷新</el-button>
        <el-button type="primary" @click="openCreate">新建</el-button>
      </div>
    </header>

    <el-table :data="items" v-loading="loading" stripe>
      <el-table-column prop="id" label="ID" width="70" />
      <el-table-column prop="type" label="类型" width="100" />
      <el-table-column prop="name" label="名称" min-width="180" show-overflow-tooltip />
      <el-table-column label="来源" width="140">
        <template #default="{ row }">
          <el-tag size="small" type="info">{{ rowSource(row) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="有效期" min-width="190">
        <template #default="{ row }">
          <el-tag size="small" :type="expiryTagType(row)">{{ expiryLabel(row) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="重复" width="110">
        <template #default="{ row }">
          <el-tag v-if="row.duplicate_of_id" size="small" type="warning">
            {{ duplicateLabel(row) }}
          </el-tag>
          <span v-else class="muted">-</span>
        </template>
      </el-table-column>
      <el-table-column label="状态" width="140">
        <template #default="{ row }"><StatusTag :status="row.status" /></template>
      </el-table-column>
      <el-table-column label="最近任务" min-width="260">
        <template #default="{ row }">
          <div v-if="latestKnowledgeJob(row)" class="job-cell">
            <div class="job-line">
              <span>{{ jobTypeLabel(latestKnowledgeJob(row).job_type) }}</span>
              <el-tag size="small" :type="jobStatusType(latestKnowledgeJob(row).status)">
                {{ jobStatusLabel(latestKnowledgeJob(row).status) }}
              </el-tag>
              <span class="muted small">
                {{ latestKnowledgeJob(row).retry_count }}/{{ latestKnowledgeJob(row).max_retries }}
              </span>
            </div>
            <span
              v-if="row.status === 'failed' && latestKnowledgeJob(row).last_error"
              class="muted danger small"
            >{{ compactJobError(latestKnowledgeJob(row)) }}</span>
          </div>
          <span v-else class="muted">-</span>
        </template>
      </el-table-column>
      <el-table-column label="更新时间" width="180">
        <template #default="{ row }">{{ formatDate(row.updated_at) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="320">
        <template #default="{ row }">
          <el-button size="small" @click="openEdit(row)">编辑</el-button>
          <el-button size="small" @click="showImpactedTasks(row)">影响任务</el-button>
          <el-button size="small" :disabled="row.status === 'processing'" @click="reprocess(row)">
            重新处理
          </el-button>
          <el-button size="small" type="danger" @click="remove(row)">删除</el-button>
        </template>
      </el-table-column>
      <template #empty>暂无知识条目，点击右上角新建。</template>
    </el-table>

    <el-dialog
      v-model="impactDialogVisible"
      :title="impactKnowledge ? `受影响任务：${impactKnowledge.name}` : '受影响任务'"
      width="720px"
    >
      <el-table :data="impactRows" v-loading="impactLoading" stripe>
        <el-table-column prop="task_id" label="任务 ID" width="110" />
        <el-table-column prop="status" label="状态" width="140" />
        <el-table-column prop="section_count" label="章节" width="90" />
        <el-table-column prop="case_count" label="用例" width="90" />
        <el-table-column label="最近追溯" min-width="180">
          <template #default="{ row }">{{ formatDate(row.last_source_context_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="100">
          <template #default="{ row }">
            <el-button size="small" @click="openTask(row)">打开</el-button>
          </template>
        </el-table-column>
        <template #empty>暂无任务在 source_context 中引用该知识。</template>
      </el-table>
    </el-dialog>

    <el-dialog
      v-model="dialogVisible"
      :title="editing ? '编辑知识' : '新建知识'"
      width="640px"
      :close-on-click-modal="false"
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-width="100px">
        <el-form-item label="类型" prop="type">
          <el-radio-group v-model="form.type" :disabled="!!editing">
            <el-radio value="product">product</el-radio>
            <el-radio value="module">module</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="名称" prop="name">
          <el-input v-model="form.name" maxlength="120" show-word-limit />
        </el-form-item>
        <el-form-item label="来源">
          <el-input v-model="form.source" maxlength="64" placeholder="manual / fixture / suggestion" />
        </el-form-item>
        <el-form-item label="过期时间">
          <el-date-picker
            v-model="form.expires_at"
            type="datetime"
            clearable
            value-format="YYYY-MM-DDTHH:mm:ssZ"
            placeholder="留空表示长期有效"
          />
        </el-form-item>
        <el-form-item label="重复指向">
          <el-input
            v-model="form.duplicate_of_id"
            clearable
            placeholder="canonical 知识 ID，留空表示非重复"
          />
          <p class="muted small">标记后该条目不再参与知识检索。</p>
        </el-form-item>
        <el-form-item label="内容" prop="content">
          <el-input
            v-model="form.content"
            type="textarea"
            :rows="10"
            placeholder="markdown 内容；保存后会自动重新向量化"
          />
        </el-form-item>
        <el-form-item label="metadata">
          <el-input
            v-model="form.metadata"
            type="textarea"
            :rows="4"
            placeholder='例如：{"aliases":["billing","账单"]}'
          />
          <p class="muted small">JSON 对象，留空表示无 metadata。</p>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="submit">保存</el-button>
      </template>
    </el-dialog>
  </section>
</template>

<style scoped>
.knowledge {
  background: #fff;
  border-radius: 8px;
  padding: 24px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
}
.bar {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  margin-bottom: 16px;
  gap: 16px;
}
.bar h2 {
  margin: 0 0 4px;
  font-size: 18px;
}
.hint {
  margin: 0;
  color: #909399;
  font-size: 13px;
}
.actions {
  display: flex;
  gap: 8px;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: wrap;
  max-width: 860px;
}
.source-filter {
  width: 150px;
}
.state-filter {
  width: 130px;
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
  margin: 4px 0 0;
}
</style>
