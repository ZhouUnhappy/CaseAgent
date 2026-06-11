<script setup>
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { bootstrapDemo, freshDemo, resetDemo } from '../api/demo'
import { useTenantStore } from '../stores/tenant'
import { notifySuccess } from '../utils/error'

const router = useRouter()
const tenantStore = useTenantStore()
const runToken = ref('')
const loadingAction = ref('')
const bootstrapResult = ref(null)
const resetResult = ref(null)

const currentTenant = computed(() => tenantStore.currentSlug || '')
const busy = computed(() => loadingAction.value !== '')
const result = computed(() => bootstrapResult.value)
const resetSummary = computed(() => bootstrapResult.value?.reset || resetResult.value)

async function runReset() {
  loadingAction.value = 'reset'
  try {
    resetResult.value = await resetDemo()
    bootstrapResult.value = null
    notifySuccess('Demo 数据已清理')
  } catch {
    /* api/client.js 已弹错 */
  } finally {
    loadingAction.value = ''
  }
}

async function runBootstrap() {
  loadingAction.value = 'bootstrap'
  try {
    bootstrapResult.value = await bootstrapDemo({ run_token: runToken.value.trim() || undefined })
    resetResult.value = null
    notifySuccess('Demo 数据已创建')
  } catch {
    /* api/client.js 已弹错 */
  } finally {
    loadingAction.value = ''
  }
}

async function runFresh() {
  loadingAction.value = 'fresh'
  try {
    bootstrapResult.value = await freshDemo({ run_token: runToken.value.trim() || undefined })
    resetResult.value = null
    notifySuccess('Demo 数据已重置并创建')
  } catch {
    /* api/client.js 已弹错 */
  } finally {
    loadingAction.value = ''
  }
}

function openProject() {
  if (!result.value?.project_id) return
  router.push({ name: 'project-detail', params: { id: result.value.project_id } }).catch(() => {})
}

function openTask() {
  if (!result.value?.task_id) return
  router.push({ name: 'task-detail', params: { id: result.value.task_id } }).catch(() => {})
}
</script>

<template>
  <section class="demo-console">
    <header class="bar">
      <div>
        <h2>Demo 控制台</h2>
        <p class="hint">使用仓库公开 fixture 为当前 tenant 创建稳定演示数据。</p>
      </div>
      <el-tag type="info">tenant: {{ currentTenant || '-' }}</el-tag>
    </header>

    <div class="toolbar">
      <el-input
        v-model="runToken"
        class="run-token"
        clearable
        placeholder="run token，可留空"
      />
      <el-button
        type="primary"
        :disabled="!currentTenant || busy"
        :loading="loadingAction === 'fresh'"
        @click="runFresh"
      >
        Reset + Bootstrap
      </el-button>
      <el-button
        :disabled="!currentTenant || busy"
        :loading="loadingAction === 'bootstrap'"
        @click="runBootstrap"
      >
        Bootstrap
      </el-button>
      <el-button
        type="danger"
        plain
        :disabled="!currentTenant || busy"
        :loading="loadingAction === 'reset'"
        @click="runReset"
      >
        Reset
      </el-button>
    </div>

    <el-alert
      v-if="!currentTenant"
      type="warning"
      title="请先在右上角选择 tenant。"
      :closable="false"
      show-icon
    />

    <div v-if="result" class="result-panel">
      <div class="result-header">
        <div>
          <h3>Bootstrap Result</h3>
          <p class="hint">后台已创建 analyze job；打开 task 后可继续 review / generate。</p>
        </div>
        <div class="result-actions">
          <el-button @click="openProject">打开项目</el-button>
          <el-button type="primary" @click="openTask">打开任务</el-button>
        </div>
      </div>

      <dl class="result-grid">
        <div>
          <dt>run_token</dt>
          <dd>{{ result.run_token }}</dd>
        </div>
        <div>
          <dt>project_id</dt>
          <dd>{{ result.project_id }}</dd>
        </div>
        <div>
          <dt>document_id</dt>
          <dd>{{ result.document_id }}</dd>
        </div>
        <div>
          <dt>task_id</dt>
          <dd>{{ result.task_id }}</dd>
        </div>
        <div>
          <dt>knowledge_ids</dt>
          <dd>{{ (result.knowledge_ids || []).join(', ') }}</dd>
        </div>
        <div>
          <dt>tenant</dt>
          <dd>{{ result.tenant_slug }}</dd>
        </div>
      </dl>

      <div class="links">
        <span>Project URL</span>
        <el-link :href="result.project_url" target="_blank">{{ result.project_url }}</el-link>
        <span>Task URL</span>
        <el-link :href="result.task_url" target="_blank">{{ result.task_url }}</el-link>
      </div>
    </div>

    <div v-if="resetSummary" class="result-panel compact">
      <h3>Reset Summary</h3>
      <dl class="result-grid">
        <div>
          <dt>matched_projects</dt>
          <dd>{{ resetSummary.matched_projects }}</dd>
        </div>
        <div>
          <dt>deleted_projects</dt>
          <dd>{{ resetSummary.deleted_projects }}</dd>
        </div>
        <div>
          <dt>matched_knowledge</dt>
          <dd>{{ resetSummary.matched_knowledge }}</dd>
        </div>
        <div>
          <dt>deleted_knowledge</dt>
          <dd>{{ resetSummary.deleted_knowledge }}</dd>
        </div>
      </dl>
    </div>
  </section>
</template>

<style scoped>
.demo-console {
  background: #fff;
  border-radius: 8px;
  padding: 24px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
}
.bar,
.toolbar,
.result-header,
.result-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}
.bar,
.result-header {
  justify-content: space-between;
}
.bar {
  margin-bottom: 16px;
}
.bar h2,
.result-panel h3 {
  margin: 0 0 4px;
  font-size: 18px;
}
.hint {
  margin: 0;
  color: #909399;
  font-size: 13px;
}
.toolbar {
  flex-wrap: wrap;
  margin-bottom: 16px;
}
.run-token {
  width: 260px;
}
.result-panel {
  margin-top: 16px;
  border: 1px solid #ebeef5;
  border-radius: 8px;
  padding: 16px;
}
.result-panel.compact {
  padding-top: 14px;
}
.result-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 12px;
  margin: 16px 0 0;
}
.result-grid div {
  min-width: 0;
}
.result-grid dt {
  color: #909399;
  font-size: 12px;
}
.result-grid dd {
  margin: 4px 0 0;
  font-weight: 650;
  word-break: break-all;
}
.links {
  display: grid;
  grid-template-columns: 110px minmax(0, 1fr);
  gap: 8px 12px;
  align-items: center;
  margin-top: 16px;
}
.links span {
  color: #909399;
  font-size: 12px;
}
@media (max-width: 760px) {
  .bar,
  .result-header {
    align-items: flex-start;
    flex-direction: column;
  }
  .run-token,
  .toolbar .el-button {
    width: 100%;
  }
  .links {
    grid-template-columns: 1fr;
  }
}
</style>
