<script setup>
import { computed } from 'vue'
import { useRoute, RouterView } from 'vue-router'

const route = useRoute()
const pageTitle = computed(() => route.meta?.title || 'CaseAgent')

const navItems = [
  { index: '/projects', label: '项目' },
  { index: '/knowledge', label: '知识库' },
]
</script>

<template>
  <el-container class="layout-root">
    <el-aside width="220px" class="layout-aside">
      <div class="brand">CaseAgent</div>
      <el-menu
        :default-active="$route.path"
        router
        class="aside-menu"
      >
        <el-menu-item
          v-for="item in navItems"
          :key="item.index"
          :index="item.index"
        >
          {{ item.label }}
        </el-menu-item>
      </el-menu>
    </el-aside>
    <el-container>
      <el-header class="layout-header">
        <div class="header-title">{{ pageTitle }}</div>
      </el-header>
      <el-main class="layout-main">
        <RouterView />
      </el-main>
    </el-container>
  </el-container>
</template>

<style scoped>
.layout-root {
  min-height: 100vh;
}
.layout-aside {
  background: #1f2329;
  color: #e5eaf3;
}
.brand {
  height: 60px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 20px;
  font-weight: 600;
  letter-spacing: 1px;
  border-bottom: 1px solid #2a2f37;
}
.aside-menu {
  border-right: none;
  background: transparent;
}
:deep(.el-menu) {
  background: transparent;
}
:deep(.el-menu-item) {
  color: #cfd3dc;
}
:deep(.el-menu-item.is-active) {
  color: #409eff;
  background: #2a2f37;
}
.layout-header {
  background: #fff;
  border-bottom: 1px solid #e4e7ed;
  display: flex;
  align-items: center;
  padding: 0 24px;
}
.header-title {
  font-size: 18px;
  font-weight: 500;
}
.layout-main {
  background: #f5f7fa;
  padding: 24px;
}
</style>
