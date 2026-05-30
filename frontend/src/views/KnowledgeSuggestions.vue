<script setup>
import { onMounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import { storeToRefs } from 'pinia'
import StatusTag from '../components/StatusTag.vue'
import { useKnowledgeSuggestionsStore } from '../stores/knowledgeSuggestions'
import { notifySuccess } from '../utils/error'

const router = useRouter()
const store = useKnowledgeSuggestionsStore()
const { items, loading, saving, statusFilter } = storeToRefs(store)

onMounted(() => store.fetch().catch(() => {}))

function formatDate(value) {
  return value ? new Date(value).toLocaleString() : '-'
}

function snippetText(snippet) {
  if (!snippet) return ''
  if (typeof snippet === 'string') return snippet
  return snippet.text || JSON.stringify(snippet)
}

const filterOptions = [
  { value: 'pending', label: '待处理' },
  { value: 'adopted', label: '已采纳' },
  { value: 'dismissed', label: '已忽略' },
  { value: '', label: '全部' },
]

const totalPending = computed(() => items.value.filter((s) => s.status === 'pending').length)

async function adopt(row) {
  // 跳转到知识库页，预填 type + name。保存知识条目后再回填 adopted + knowledge id。
  router.push({
    name: 'knowledge',
    query: {
      create_type: row.candidate_type,
      create_name: row.candidate_name,
      from_suggestion_id: row.id,
    },
  })
  notifySuccess('请填写内容并保存，保存后会标记为已采纳')
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
          analyze 阶段在需求中识别出但当前知识库未覆盖（retrieval top-1 score &lt; 0.5）的候选词。
          采纳会跳转到知识库页预填新建对话框；忽略则把候选标记为 dismissed。
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
        <el-button @click="store.fetch()" :loading="loading">刷新</el-button>
      </div>
    </header>

    <p v-if="statusFilter === 'pending'" class="muted small">
      当前待处理：{{ totalPending }} 条
    </p>

    <el-table :data="items" v-loading="loading" stripe>
      <el-table-column prop="id" label="ID" width="80" />
      <el-table-column label="类型" width="100">
        <template #default="{ row }">
          <el-tag size="small" :type="row.candidate_type === 'product' ? 'warning' : 'info'">
            {{ row.candidate_type }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="candidate_name" label="候选名称" min-width="180" show-overflow-tooltip />
      <el-table-column label="频次" width="80">
        <template #default="{ row }">{{ row.frequency }}</template>
      </el-table-column>
      <el-table-column label="来源 task" width="120">
        <template #default="{ row }">
          <router-link :to="{ name: 'task-detail', params: { id: row.source_task_id } }">
            #{{ row.source_task_id }}
          </router-link>
        </template>
      </el-table-column>
      <el-table-column label="状态" width="120">
        <template #default="{ row }"><StatusTag :status="row.status" /></template>
      </el-table-column>
      <el-table-column label="发现时间" width="180">
        <template #default="{ row }">{{ formatDate(row.created_at) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="180" align="center">
        <template #default="{ row }">
          <div v-if="row.status === 'pending'" class="op-buttons">
            <el-button size="small" type="primary" :loading="saving" @click="adopt(row)">
              采纳
            </el-button>
            <el-button size="small" :loading="saving" @click="dismiss(row)">忽略</el-button>
          </div>
          <span v-else class="muted">—</span>
        </template>
      </el-table-column>
      <el-table-column type="expand" width="50">
        <template #default="{ row }">
          <div class="snippet-list">
            <strong>上下文片段：</strong>
            <ul v-if="(row.source_snippets || []).length">
              <li v-for="(s, idx) in row.source_snippets" :key="idx">{{ snippetText(s) }}</li>
            </ul>
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
.snippet-list ul {
  margin: 4px 0 0 16px;
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
