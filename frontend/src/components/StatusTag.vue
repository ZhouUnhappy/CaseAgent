<script setup>
import { computed } from 'vue'

// StatusTag renders backend status strings consistently. We do NOT derive
// or override the status text — it is taken verbatim from the API response,
// per I3-T2 ("页面展示的状态字段直接来自后端响应，前端不本地推断").
const props = defineProps({
  status: { type: String, required: true },
})

const META = {
  pending: { type: 'info', label: 'pending' },
  processing: { type: 'warning', label: 'processing' },
  completed: { type: 'success', label: 'completed' },
  failed: { type: 'danger', label: 'failed' },

  analyzing: { type: 'warning', label: 'analyzing' },
  awaiting_review: { type: 'info', label: 'awaiting_review' },
  ready_to_generate: { type: 'info', label: 'ready_to_generate' },
  generating: { type: 'warning', label: 'generating' },

  draft: { type: 'info', label: 'draft' },
  submitted: { type: 'success', label: 'submitted' },
  approved: { type: 'success', label: 'approved' },
}

const meta = computed(() => META[props.status] || { type: '', label: props.status || '-' })
</script>

<template>
  <el-tag :type="meta.type" disable-transitions>{{ meta.label }}</el-tag>
</template>
