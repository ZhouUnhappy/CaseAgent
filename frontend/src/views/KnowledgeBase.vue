<script setup>
import { onMounted, reactive, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { ElMessageBox } from 'element-plus'
import StatusTag from '../components/StatusTag.vue'
import { useKnowledgeStore } from '../stores/knowledge'
import { notifySuccess } from '../utils/error'

const store = useKnowledgeStore()
const { items, loading, saving, typeFilter } = storeToRefs(store)

const dialogVisible = ref(false)
const editing = ref(null)
const form = reactive({ type: 'product', name: '', content: '', metadata: '' })
const formRef = ref(null)

const rules = {
  type: [{ required: true, message: '请选择类型', trigger: 'change' }],
  name: [{ required: true, message: '请输入名称', trigger: 'blur' }],
  content: [{ required: true, message: '请输入内容', trigger: 'blur' }],
}

onMounted(() => store.fetch().catch(() => {}))

function formatDate(value) {
  return value ? new Date(value).toLocaleString() : '-'
}

function openCreate() {
  editing.value = null
  Object.assign(form, { type: 'product', name: '', content: '', metadata: '' })
  dialogVisible.value = true
}

function openEdit(row) {
  editing.value = row
  form.type = row.type
  form.name = row.name
  form.content = row.content
  form.metadata = row.metadata ? JSON.stringify(row.metadata, null, 2) : ''
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

async function submit() {
  if (!formRef.value) return
  const ok = await formRef.value.validate().catch(() => false)
  if (!ok) return
  let metadata = null
  try {
    metadata = parseMetadata()
  } catch (err) {
    ElMessageBox.alert(err.message)
    return
  }
  const payload = { type: form.type, name: form.name, content: form.content, metadata }
  try {
    if (editing.value) {
      await store.update(editing.value.id, {
        name: payload.name,
        content: payload.content,
        metadata: payload.metadata,
      })
      notifySuccess('知识条目已更新')
    } else {
      await store.create(payload)
      notifySuccess('知识条目已创建，后台正在向量化')
    }
    dialogVisible.value = false
  } catch {
    /* api/client.js 已弹错 */
  }
}

async function reprocess(row) {
  try {
    await store.reprocess(row.id)
    notifySuccess(`知识 ${row.name} 重新向量化已触发`)
  } catch {
    /* 错误已弹窗 */
  }
}

async function remove(row) {
  try {
    await ElMessageBox.confirm(`删除知识条目「${row.name}」？`, '确认', { type: 'warning' })
  } catch {
    return
  }
  try {
    await store.remove(row.id)
    notifySuccess('已删除')
  } catch {
    /* 错误已弹窗 */
  }
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
          @change="(v) => store.setTypeFilter(v).catch(() => {})"
        >
          <el-radio-button value="">全部</el-radio-button>
          <el-radio-button value="product">product</el-radio-button>
          <el-radio-button value="module">module</el-radio-button>
        </el-radio-group>
        <el-button @click="store.fetch()" :loading="loading">刷新</el-button>
        <el-button type="primary" @click="openCreate">新建</el-button>
      </div>
    </header>

    <el-table :data="items" v-loading="loading" stripe>
      <el-table-column prop="id" label="ID" width="70" />
      <el-table-column prop="type" label="类型" width="100" />
      <el-table-column prop="name" label="名称" min-width="180" show-overflow-tooltip />
      <el-table-column label="状态" width="140">
        <template #default="{ row }"><StatusTag :status="row.status" /></template>
      </el-table-column>
      <el-table-column label="更新时间" width="180">
        <template #default="{ row }">{{ formatDate(row.updated_at) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="240">
        <template #default="{ row }">
          <el-button size="small" @click="openEdit(row)">编辑</el-button>
          <el-button size="small" :disabled="row.status === 'processing'" @click="reprocess(row)">
            重新处理
          </el-button>
          <el-button size="small" type="danger" @click="remove(row)">删除</el-button>
        </template>
      </el-table-column>
      <template #empty>暂无知识条目，点击右上角新建。</template>
    </el-table>

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
}
.muted {
  color: #909399;
}
.small {
  font-size: 12px;
  margin: 4px 0 0;
}
</style>
