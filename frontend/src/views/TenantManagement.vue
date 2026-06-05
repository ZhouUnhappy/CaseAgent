<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { ElMessageBox } from 'element-plus'
import { Check, EditPen, FolderOpened, Plus, Refresh, Switch } from '@element-plus/icons-vue'
import { useTenantStore } from '../stores/tenant'
import { notifySuccess } from '../utils/error'

const tenantStore = useTenantStore()
const { items: tenants, currentSlug, defaultSlug, loading, creating, savingSlug } = storeToRefs(tenantStore)

const createVisible = ref(false)
const editVisible = ref(false)
const createForm = reactive({ slug: '', name: '' })
const editForm = reactive({ slug: '', name: '' })
const showArchived = ref(true)

const visibleTenants = computed(() =>
  showArchived.value ? tenants.value : tenants.value.filter((tenant) => !tenant.archived_at),
)

onMounted(() => {
  refreshTenants()
})

function formatDate(value) {
  return value ? new Date(value).toLocaleString() : '-'
}

function refreshTenants() {
  tenantStore.fetch({ includeArchived: true }).catch(() => {})
}

function openCreate() {
  Object.assign(createForm, { slug: '', name: '' })
  createVisible.value = true
}

async function submitCreate() {
  if (!createForm.slug.trim() || !createForm.name.trim()) {
    await ElMessageBox.alert('slug 和名称都必填')
    return
  }
  try {
    const created = await tenantStore.create({
      slug: createForm.slug.trim(),
      name: createForm.name.trim(),
    })
    createVisible.value = false
    tenantStore.setCurrent(created.slug)
    notifySuccess(`租户 ${created.slug} 已创建`)
  } catch {
    /* api/client.js 已弹错 */
  }
}

function openEdit(row) {
  Object.assign(editForm, { slug: row.slug, name: row.name })
  editVisible.value = true
}

async function submitEdit() {
  if (!editForm.name.trim()) {
    await ElMessageBox.alert('名称必填')
    return
  }
  try {
    await tenantStore.update(editForm.slug, { name: editForm.name.trim() })
    editVisible.value = false
    notifySuccess(`租户 ${editForm.slug} 已更新`)
  } catch {
    /* api/client.js 已弹错 */
  }
}

async function archive(row) {
  try {
    await ElMessageBox.confirm(`归档租户 ${row.slug}？归档后业务 API 将不再接受该 tenant。`, '确认归档', {
      type: 'warning',
    })
  } catch {
    return
  }
  try {
    await tenantStore.archive(row.slug)
    notifySuccess(`租户 ${row.slug} 已归档`)
  } catch {
    /* api/client.js 已弹错 */
  }
}

async function unarchive(row) {
  try {
    await tenantStore.unarchive(row.slug)
    notifySuccess(`租户 ${row.slug} 已恢复`)
  } catch {
    /* api/client.js 已弹错 */
  }
}

function selectTenant(row) {
  tenantStore.setCurrent(row.slug)
  notifySuccess(`已切换到 ${row.slug}`)
}

function setDefault(row) {
  tenantStore.setDefault(row.slug)
  notifySuccess(`默认租户已设为 ${row.slug}`)
}
</script>

<template>
  <section class="tenant-management">
    <header class="page-header">
      <div>
        <h2>租户管理</h2>
        <p class="muted">管理 demo / 试用环境的 tenant 生命周期与默认入口。</p>
      </div>
      <div class="header-actions">
        <el-switch v-model="showArchived" active-text="显示归档" />
        <el-button :icon="Refresh" :loading="loading" @click="refreshTenants">刷新</el-button>
        <el-button type="primary" :icon="Plus" @click="openCreate">新建</el-button>
      </div>
    </header>

    <el-table :data="visibleTenants" v-loading="loading" stripe class="tenant-table">
      <el-table-column prop="slug" label="Slug" min-width="150" />
      <el-table-column prop="name" label="名称" min-width="180" />
      <el-table-column label="状态" width="110">
        <template #default="{ row }">
          <el-tag :type="row.archived_at ? 'info' : 'success'" size="small">
            {{ row.archived_at ? 'archived' : 'active' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="标记" width="160">
        <template #default="{ row }">
          <el-tag v-if="row.slug === currentSlug" type="primary" size="small" class="chip">当前</el-tag>
          <el-tag v-if="row.slug === defaultSlug" type="warning" size="small" class="chip">默认</el-tag>
          <span v-if="row.slug !== currentSlug && row.slug !== defaultSlug" class="muted">-</span>
        </template>
      </el-table-column>
      <el-table-column label="创建时间" min-width="180">
        <template #default="{ row }">{{ formatDate(row.created_at) }}</template>
      </el-table-column>
      <el-table-column label="归档时间" min-width="180">
        <template #default="{ row }">{{ formatDate(row.archived_at) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="280" fixed="right">
        <template #default="{ row }">
          <el-tooltip content="切换到该租户" placement="top">
            <el-button
              :icon="Switch"
              size="small"
              circle
              :disabled="Boolean(row.archived_at) || row.slug === currentSlug"
              @click="selectTenant(row)"
            />
          </el-tooltip>
          <el-tooltip content="设为默认" placement="top">
            <el-button
              :icon="Check"
              size="small"
              circle
              :disabled="Boolean(row.archived_at) || row.slug === defaultSlug"
              @click="setDefault(row)"
            />
          </el-tooltip>
          <el-tooltip content="重命名" placement="top">
            <el-button :icon="EditPen" size="small" circle :loading="savingSlug === row.slug" @click="openEdit(row)" />
          </el-tooltip>
          <el-tooltip :content="row.archived_at ? '恢复' : '归档'" placement="top">
            <el-button
              :icon="FolderOpened"
              size="small"
              circle
              :type="row.archived_at ? 'success' : 'warning'"
              :loading="savingSlug === row.slug"
              @click="row.archived_at ? unarchive(row) : archive(row)"
            />
          </el-tooltip>
        </template>
      </el-table-column>
      <template #empty>暂无租户</template>
    </el-table>

    <el-dialog v-model="createVisible" title="新建租户" width="420px">
      <el-form label-width="80px">
        <el-form-item label="slug" required>
          <el-input v-model="createForm.slug" placeholder="例如 demo-caseagent" />
        </el-form-item>
        <el-form-item label="名称" required>
          <el-input v-model="createForm.name" placeholder="人类可读名称" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createVisible = false">取消</el-button>
        <el-button type="primary" :loading="creating" @click="submitCreate">创建</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="editVisible" title="重命名租户" width="420px">
      <el-form label-width="80px">
        <el-form-item label="slug">
          <el-input v-model="editForm.slug" disabled />
        </el-form-item>
        <el-form-item label="名称" required>
          <el-input v-model="editForm.name" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editVisible = false">取消</el-button>
        <el-button type="primary" :loading="savingSlug === editForm.slug" @click="submitEdit">保存</el-button>
      </template>
    </el-dialog>
  </section>
</template>

<style scoped>
.tenant-management {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.page-header {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  align-items: flex-start;
}
.page-header h2 {
  margin: 0 0 6px;
  font-size: 20px;
  font-weight: 700;
}
.header-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 10px;
  flex-wrap: wrap;
}
.tenant-table {
  width: 100%;
}
.muted {
  color: #64748b;
  font-size: 13px;
}
.chip {
  margin-right: 4px;
}
</style>
