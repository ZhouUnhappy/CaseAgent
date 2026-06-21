<script setup>
import { onMounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import { storeToRefs } from 'pinia'
import StatusTag from '../components/StatusTag.vue'
import { useKnowledgeSuggestionsStore } from '../stores/knowledgeSuggestions'
import { writeSuggestionDraft } from '../utils/knowledgeSuggestionDraft'
import { notifySuccess } from '../utils/error'
import { knowledgeTypeLabel } from '../utils/labels'

const router = useRouter()
const store = useKnowledgeSuggestionsStore()
const { items, loading, saving, draftingId, statusFilter, showAutoExpired } = storeToRefs(store)

onMounted(() => store.fetch().catch(() => {}))

function formatDate(value) {
  return value ? new Date(value).toLocaleString() : '-'
}

function snippetText(snippet) {
  if (!snippet) return ''
  if (typeof snippet === 'string') return snippet
  if (snippet.type === 'failure') {
    return `失败阶段：${snippet.stage || '-'}；错误：${snippet.error || '-'}`
  }
  if (snippet.type === 'affected_scope') {
    const products = Array.isArray(snippet.affected_products) ? snippet.affected_products.join(', ') : ''
    const modules = Array.isArray(snippet.affected_modules) ? snippet.affected_modules.join(', ') : ''
    const documents = Array.isArray(snippet.document_ids) ? snippet.document_ids.join(', ') : ''
    return `影响范围：产品=${products || '-'}；模块=${modules || '-'}；文档=${documents || '-'}`
  }
  if (snippet.type === 'knowledge_context') {
    const ids = Array.isArray(snippet.knowledge_ids) ? snippet.knowledge_ids.join(', ') : ''
    const names = Array.isArray(snippet.knowledge_names) ? snippet.knowledge_names.join(', ') : ''
    return `相关知识：ids=${ids || '-'}；names=${names || '-'}`
  }
  return snippet.text || JSON.stringify(snippet)
}

function candidateTagType(type) {
  if (type === 'product') return 'warning'
  if (type === 'context_gap') return 'danger'
  return 'info'
}

function isContextGap(row) {
  return row?.candidate_type === 'context_gap'
}

function rowOccurrences(row) {
  return Array.isArray(row.occurrences) ? row.occurrences : []
}

function rowTotalFrequency(row) {
  return row.total_frequency ?? row.frequency ?? 0
}

function occurrenceSnippets(occurrence) {
  return Array.isArray(occurrence?.source_snippets) ? occurrence.source_snippets : []
}

function occurrenceKey(occurrence, idx) {
  const base = occurrence.id || `${occurrence.source_task_id}-${occurrence.source_case_id || 'task'}`
  return `${base}-${idx}`
}

const filterOptions = [
  { value: 'pending', label: '待处理' },
  { value: 'adopted', label: '已采纳' },
  { value: 'dismissed', label: '已忽略' },
  { value: '', label: '全部' },
]

const totalPending = computed(() => items.value.filter((s) => s.status === 'pending').length)
const visibleItems = computed(() => {
  if (showAutoExpired.value) return items.value
  return items.value.filter((s) => s.dismissed_reason !== 'auto_expired')
})
const hiddenAutoExpiredCount = computed(
  () => items.value.length - visibleItems.value.length,
)

async function adopt(row) {
  if (isContextGap(row)) return
  try {
    const draft = await store.draft(row.id)
    writeSuggestionDraft(row.id, draft?.draft_content || '')
    router.push({
      name: 'knowledge',
      query: {
        type: row.candidate_type,
        name: row.candidate_name,
        from_suggestion_id: row.id,
      },
    })
    notifySuccess(draft?.draft_content ? '草稿已生成，请校对后保存' : '请填写内容并保存，保存后会标记为已采纳')
  } catch {
    /* 错误已弹窗 */
  }
}

async function dismiss(row) {
  try {
    await store.setStatus(row.id, 'dismissed')
    notifySuccess('已忽略')
  } catch {
    /* 错误已弹窗 */
  }
}
</script>

<template>
  <section class="suggestions">
    <header class="bar">
      <div>
        <h2>知识建议</h2>
        <p class="hint">
          按候选聚合，优先处理跨任务反复出现的知识缺口。
        </p>
      </div>
      <div class="actions">
        <el-radio-group
          :model-value="statusFilter"
          @change="(v) => store.setStatusFilter(v).catch(() => {})"
        >
          <el-radio-button
            v-for="opt in filterOptions"
            :key="opt.value"
            :value="opt.value"
          >{{ opt.label }}</el-radio-button>
        </el-radio-group>
        <el-checkbox
          :model-value="showAutoExpired"
          @change="(v) => store.setShowAutoExpired(v)"
        >显示自动过期</el-checkbox>
        <el-button @click="store.fetch()" :loading="loading">刷新</el-button>
      </div>
    </header>

    <p v-if="statusFilter === 'pending'" class="muted small">
      当前待处理：{{ totalPending }} 条
    </p>
    <p v-else-if="hiddenAutoExpiredCount > 0" class="muted small">
      已隐藏自动过期：{{ hiddenAutoExpiredCount }} 条
    </p>

    <el-table :data="visibleItems" v-loading="loading" stripe>
      <el-table-column prop="id" label="ID" width="80" />
      <el-table-column label="类型" width="100">
        <template #default="{ row }">
          <el-tag size="small" :type="candidateTagType(row.candidate_type)">
            {{ knowledgeTypeLabel(row.candidate_type) }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="candidate_name" label="候选名称" min-width="180" show-overflow-tooltip />
      <el-table-column label="任务数" width="90">
        <template #default="{ row }">{{ row.task_count || 0 }}</template>
      </el-table-column>
      <el-table-column label="总频次" width="90">
        <template #default="{ row }">{{ rowTotalFrequency(row) }}</template>
      </el-table-column>
      <el-table-column label="最新来源" width="120">
        <template #default="{ row }">
          <router-link v-if="row.source_task_id" :to="{ name: 'task-detail', params: { id: row.source_task_id } }">
            #{{ row.source_task_id }}
          </router-link>
          <span v-else class="muted">-</span>
        </template>
      </el-table-column>
      <el-table-column label="状态" width="120">
        <template #default="{ row }"><StatusTag :status="row.status" /></template>
      </el-table-column>
      <el-table-column label="忽略原因" width="120">
        <template #default="{ row }">
          <el-tag v-if="row.dismissed_reason === 'auto_expired'" size="small" type="info">
            自动过期
          </el-tag>
          <span v-else class="muted">-</span>
        </template>
      </el-table-column>
      <el-table-column label="最后发现" width="180">
        <template #default="{ row }">{{ formatDate(row.last_seen_at || row.updated_at || row.created_at) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="180" align="center">
        <template #default="{ row }">
          <div v-if="row.status === 'pending'" class="op-buttons">
            <el-tooltip
              v-if="isContextGap(row)"
              content="上下文缺失需要先排查失败原因，暂不自动生成知识草稿"
              placement="top"
            >
              <span>
                <el-button size="small" type="primary" disabled>采纳</el-button>
              </span>
            </el-tooltip>
            <el-button
              v-else
              size="small"
              type="primary"
              :loading="draftingId === row.id"
              @click="adopt(row)"
            >
              采纳
            </el-button>
            <el-button size="small" :loading="saving" @click="dismiss(row)">忽略</el-button>
          </div>
          <span v-else class="muted">—</span>
        </template>
      </el-table-column>
      <el-table-column type="expand" width="50">
        <template #default="{ row }">
          <div class="occurrence-panel">
            <div class="occurrence-header">
              <strong>出现明细</strong>
              <span class="muted small">{{ rowOccurrences(row).length }} 条</span>
            </div>
            <el-table
              v-if="rowOccurrences(row).length"
              :data="rowOccurrences(row)"
              size="small"
              border
            >
              <el-table-column label="Task" width="110">
                <template #default="{ row: occurrence }">
                  <router-link :to="{ name: 'task-detail', params: { id: occurrence.source_task_id } }">
                    #{{ occurrence.source_task_id }}
                  </router-link>
                </template>
              </el-table-column>
              <el-table-column label="Case" width="100">
                <template #default="{ row: occurrence }">
                  <span v-if="occurrence.source_case_id">#{{ occurrence.source_case_id }}</span>
                  <span v-else class="muted">-</span>
                </template>
              </el-table-column>
              <el-table-column label="频次" width="80">
                <template #default="{ row: occurrence }">{{ occurrence.frequency || 0 }}</template>
              </el-table-column>
              <el-table-column label="时间" width="180">
                <template #default="{ row: occurrence }">{{ formatDate(occurrence.created_at) }}</template>
              </el-table-column>
              <el-table-column label="上下文">
                <template #default="{ row: occurrence }">
                  <ul v-if="occurrenceSnippets(occurrence).length" class="snippet-list">
                    <li
                      v-for="(snippet, idx) in occurrenceSnippets(occurrence)"
                      :key="occurrenceKey(occurrence, idx)"
                    >
                      {{ snippetText(snippet) }}
                    </li>
                  </ul>
                  <span v-else class="muted">-</span>
                </template>
              </el-table-column>
            </el-table>
            <span v-else class="muted">无</span>
          </div>
        </template>
      </el-table-column>
      <template #empty>暂无建议</template>
    </el-table>
  </section>
</template>

<style scoped>
.suggestions {
  background: #fff;
  border-radius: 8px;
  padding: 24px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
}
.bar {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 16px;
  margin-bottom: 16px;
}
.bar > div:first-child {
  flex: 1 1 360px;
  min-width: 0;
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
  flex-wrap: nowrap;
  flex-shrink: 0;
}
.actions :deep(.el-radio-group) {
  flex-wrap: nowrap;
}
.muted {
  color: #909399;
}
.small {
  font-size: 12px;
}
.occurrence-panel {
  padding: 8px 12px 12px;
}
.occurrence-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}
.snippet-list {
  margin: 0 0 0 16px;
  padding: 0;
  color: #606266;
}
.op-buttons {
  display: inline-flex;
  gap: 8px;
  justify-content: center;
}
.op-buttons :deep(.el-button + .el-button) {
  margin-left: 0;
}
</style>
