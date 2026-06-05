<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRoute, RouterView } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Collection, DocumentChecked, FolderOpened, MagicStick, Monitor } from '@element-plus/icons-vue'
import { useTenantStore } from '../stores/tenant'

const route = useRoute()
const pageTitle = computed(() => route.meta?.title || 'CaseAgent')
const activeMenu = computed(() => {
  if (route.name === 'task-detail') return '/generate'
  if (route.name === 'project-detail') return '/projects'
  return route.path
})

const navItems = [
  { index: '/generate', label: '生成用例', icon: MagicStick },
  { index: '/projects', label: '项目管理', icon: FolderOpened },
  { index: '/knowledge', label: '知识库', icon: Collection },
  { index: '/knowledge-suggestions', label: '知识建议', icon: DocumentChecked },
  { index: '/ops', label: '运维', icon: Monitor },
]

const tenantStore = useTenantStore()

onMounted(() => {
  tenantStore.fetch().catch(() => {
    // notifyApiError already handles user-facing error; no-op here
  })
})

function onTenantChange(slug) {
  tenantStore.setCurrent(slug)
  // Hard reload so every view re-fetches data scoped to the new tenant
  // instead of needing each store to subscribe to tenant changes.
  window.location.reload()
}

const createDialogVisible = ref(false)
const createForm = ref({ slug: '', name: '' })

function openCreateDialog() {
  createForm.value = { slug: '', name: '' }
  createDialogVisible.value = true
}

async function submitCreate() {
  const { slug, name } = createForm.value
  if (!slug || !name) {
    ElMessage.warning('slug 和 name 都是必填')
    return
  }
  try {
    const created = await tenantStore.create({ slug, name })
    ElMessage.success(`租户 ${created.slug} 已创建`)
    createDialogVisible.value = false
    tenantStore.setCurrent(created.slug)
    window.location.reload()
  } catch {
    // notifyApiError already shown
  }
}
</script>

<template>
  <el-container class="layout-root">
    <el-aside width="220px" class="layout-aside">
      <div class="brand">
        <div class="brand-mark">CA</div>
        <div>
          <strong>CaseAgent</strong>
          <span>Test case workbench</span>
        </div>
      </div>
      <el-menu
        :default-active="activeMenu"
        router
        class="aside-menu"
      >
        <el-menu-item
          v-for="item in navItems"
          :key="item.index"
          :index="item.index"
        >
          <el-icon><component :is="item.icon" /></el-icon>
          {{ item.label }}
        </el-menu-item>
      </el-menu>
    </el-aside>
    <el-container>
      <el-header class="layout-header">
        <div class="header-title">{{ pageTitle }}</div>
        <div class="header-tenant">
          <span class="tenant-label">当前租户</span>
          <el-select
            :model-value="tenantStore.currentSlug"
            :loading="tenantStore.loading"
            placeholder="选择租户"
            size="small"
            style="width: 180px"
            @change="onTenantChange"
          >
            <el-option
              v-for="t in tenantStore.items"
              :key="t.slug"
              :label="`${t.name} (${t.slug})`"
              :value="t.slug"
            />
          </el-select>
          <el-button size="small" @click="openCreateDialog">新建</el-button>
        </div>
      </el-header>
      <el-main class="layout-main">
        <RouterView />
      </el-main>
    </el-container>
  </el-container>

  <el-dialog
    v-model="createDialogVisible"
    title="新建租户"
    width="420px"
  >
    <el-form label-width="80px">
      <el-form-item label="slug" required>
        <el-input v-model="createForm.slug" placeholder="例如 i1-smoke" />
      </el-form-item>
      <el-form-item label="名称" required>
        <el-input v-model="createForm.name" placeholder="人类可读名称" />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="createDialogVisible = false">取消</el-button>
      <el-button
        type="primary"
        :loading="tenantStore.creating"
        @click="submitCreate"
      >创建</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.layout-root {
  min-height: 100vh;
}
.layout-aside {
  background: #ffffff;
  color: #111827;
  border-right: 1px solid #e5e7eb;
}
.brand {
  height: 68px;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 18px;
  border-bottom: 1px solid #eef2f7;
  box-sizing: border-box;
}
.brand-mark {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 34px;
  height: 34px;
  border-radius: 8px;
  background: #2563eb;
  color: #fff;
  font-size: 13px;
  font-weight: 750;
}
.brand strong,
.brand span {
  display: block;
}
.brand strong {
  font-size: 16px;
  font-weight: 700;
}
.brand span {
  margin-top: 2px;
  color: #64748b;
  font-size: 11px;
}
.aside-menu {
  border-right: none;
  background: transparent;
  padding: 10px;
}
:deep(.el-menu) {
  background: transparent;
}
:deep(.el-menu-item) {
  height: 42px;
  margin-bottom: 4px;
  border-radius: 8px;
  color: #475569;
}
:deep(.el-menu-item.is-active) {
  color: #1d4ed8;
  background: #eff6ff;
  font-weight: 650;
}
.layout-header {
  background: rgba(255, 255, 255, 0.92);
  border-bottom: 1px solid #e5e7eb;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 24px;
  gap: 16px;
}
.header-title {
  font-size: 18px;
  font-weight: 650;
  color: #111827;
}
.header-tenant {
  display: flex;
  align-items: center;
  gap: 8px;
}
.tenant-label {
  color: #64748b;
  font-size: 13px;
}
.layout-main {
  background: #f6f8fb;
  padding: 24px;
}
</style>
