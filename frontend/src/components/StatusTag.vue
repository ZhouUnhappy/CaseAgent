<script setup>
import { computed } from 'vue'
import { statusMeta } from '../utils/status'

const props = defineProps({
  status: { type: String, required: true },
})

const meta = computed(() => statusMeta(props.status))
</script>

<template>
  <el-tag :type="meta.type" disable-transitions>{{ meta.label }}</el-tag>
</template>
