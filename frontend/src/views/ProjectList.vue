<script setup>
import { onMounted, reactive, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useProjectsStore } from '../stores/projects'
import { formatDateTime as formatDate } from '../utils/date'
import { notifySuccess } from '../utils/error'

const store = useProjectsStore()
const { items, loading, creating } = storeToRefs(store)

const dialogVisible = ref(false)
const form = reactive({ name: '', description: '' })
const formRef = ref(null)
const rules = {
  name: [{ required: true, message: '请输入项目名称', trigger: 'blur' }],
}

onMounted(() => {
  store.fetch().catch(() => {
    // 错误已由 api/client.js 统一弹窗，列表会保持上一次状态
  })
})

function openCreate() {
  form.name = ''
  form.description = ''
  dialogVisible.value = true
}

async function submitCreate() {
  if (!formRef.value) return
  const ok = await formRef.value.validate().catch(() => false)
  if (!ok) return
  try {
    await store.create({ name: form.name, description: form.description })
    notifySuccess('项目已创建')
    dialogVisible.value = false
  } catch {
    // 错误已弹窗
  }
}

</script>

<template>
  <section class="project-list">
    <header class="bar">
      <div>
        <h2>项目</h2>
        <p class="hint">在项目下管理参考文档、生成任务与测试用例。</p>
      </div>
      <div class="actions">
        <el-button @click="store.fetch()" :loading="loading">刷新</el-button>
        <el-button type="primary" @click="openCreate">新建项目</el-button>
      </div>
    </header>

    <el-table :data="items" v-loading="loading" stripe>
      <el-table-column prop="id" label="ID" width="80" />
      <el-table-column label="名称" min-width="180">
        <template #default="{ row }">
          <router-link :to="{ name: 'project-detail', params: { id: row.id } }">
            {{ row.name }}
          </router-link>
        </template>
      </el-table-column>
      <el-table-column prop="description" label="描述" min-width="240" show-overflow-tooltip />
      <el-table-column label="创建时间" width="200">
        <template #default="{ row }">{{ formatDate(row.created_at) }}</template>
      </el-table-column>
      <el-table-column label="更新时间" width="200">
        <template #default="{ row }">{{ formatDate(row.updated_at) }}</template>
      </el-table-column>
      <template #empty>
        <span>暂无项目，点击右上角"新建项目"开始。</span>
      </template>
    </el-table>

    <el-dialog
      v-model="dialogVisible"
      title="新建项目"
      width="480px"
      :close-on-click-modal="false"
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-width="80px">
        <el-form-item label="名称" prop="name">
          <el-input v-model="form.name" maxlength="64" show-word-limit />
        </el-form-item>
        <el-form-item label="描述">
          <el-input
            v-model="form.description"
            type="textarea"
            :rows="3"
            maxlength="240"
            show-word-limit
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="creating" @click="submitCreate">创建</el-button>
      </template>
    </el-dialog>
  </section>
</template>

<style scoped>
.project-list {
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
}
</style>
