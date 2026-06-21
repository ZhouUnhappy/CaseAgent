<script setup>
import { computed } from 'vue'
import DOMPurify from 'dompurify'
import { marked } from 'marked'

const props = defineProps({
  content: {
    type: String,
    default: '',
  },
})

const stats = computed(() => {
  const content = props.content || ''
  const lines = content ? content.split(/\r?\n/) : []
  return {
    chars: content.length,
    lines: lines.length,
    headings: lines.filter((line) => /^#{1,4}\s+/.test(line.trim())).length,
  }
})

const renderedContent = computed(() => {
  const content = props.content || ''
  if (!content) return ''
  const html = marked.parse(content, { gfm: true, breaks: true })
  return DOMPurify.sanitize(html, {
    USE_PROFILES: { html: true },
    FORBID_TAGS: ['style', 'iframe', 'form'],
  })
})
</script>

<template>
  <div class="markdown-preview">
    <div class="preview-stats">
      <span>{{ stats.chars.toLocaleString() }} 字符</span>
      <span>{{ stats.lines.toLocaleString() }} 行</span>
      <span>{{ stats.headings.toLocaleString() }} 个标题</span>
    </div>

    <div v-if="renderedContent" class="reading-surface" v-html="renderedContent" />
    <el-empty v-else description="暂无可预览内容" />

    <details v-if="content" class="source-details">
      <summary>查看 Markdown 原文</summary>
      <pre>{{ content }}</pre>
    </details>
  </div>
</template>

<style scoped>
.markdown-preview {
  color: #303133;
}

.preview-stats {
  display: flex;
  gap: 16px;
  padding: 10px 0 14px;
  color: #909399;
  font-size: 12px;
  border-bottom: 1px solid #ebeef5;
}

.reading-surface {
  max-width: 1120px;
  margin: 0 auto;
  padding: 20px 8px 32px;
  line-height: 1.75;
}

.reading-surface :deep(h1),
.reading-surface :deep(h2),
.reading-surface :deep(h3),
.reading-surface :deep(h4),
.reading-surface :deep(h5) {
  margin: 26px 0 10px;
  color: #1f2329;
  letter-spacing: 0;
}

.reading-surface :deep(h1) {
  font-size: 26px;
}

.reading-surface :deep(h2) {
  font-size: 22px;
}

.reading-surface :deep(h3) {
  font-size: 18px;
}

.reading-surface :deep(h4),
.reading-surface :deep(h5) {
  font-size: 16px;
}

.reading-surface :deep(p) {
  margin: 10px 0;
  overflow-wrap: anywhere;
}

.reading-surface :deep(ul),
.reading-surface :deep(ol) {
  margin: 10px 0;
  padding-left: 26px;
}

.reading-surface :deep(li + li) {
  margin-top: 6px;
}

.reading-surface :deep(blockquote) {
  margin: 14px 0;
  padding: 10px 14px;
  border-left: 3px solid #409eff;
  background: #f5f7fa;
  color: #606266;
}

.reading-surface :deep(pre) {
  margin: 14px 0;
  padding: 14px 16px;
  overflow: auto;
  border: 1px solid #dcdfe6;
  border-radius: 6px;
  background: #f5f7fa;
  color: #303133;
  line-height: 1.6;
  white-space: pre;
}

.reading-surface :deep(code) {
  padding: 2px 5px;
  border-radius: 4px;
  background: #f5f7fa;
  font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
  font-size: 0.92em;
}

.reading-surface :deep(pre code) {
  padding: 0;
  background: transparent;
  font-size: inherit;
}

.reading-surface :deep(a) {
  color: #409eff;
  overflow-wrap: anywhere;
}

.reading-surface :deep(table) {
  width: 100%;
  margin: 16px 0;
  border-collapse: collapse;
  font-size: 14px;
}

.reading-surface :deep(th),
.reading-surface :deep(td) {
  padding: 9px 12px;
  border: 1px solid #dcdfe6;
  text-align: left;
  vertical-align: top;
}

.reading-surface :deep(th) {
  background: #f5f7fa;
  font-weight: 600;
}

.reading-surface :deep(hr) {
  margin: 24px 0;
  border: 0;
  border-top: 1px solid #dcdfe6;
}

.source-details {
  border-top: 1px solid #ebeef5;
  padding-top: 14px;
}

.source-details summary {
  cursor: pointer;
  color: #606266;
  font-size: 13px;
}

.source-details pre {
  max-height: 360px;
  margin: 12px 0 0;
  padding: 14px;
  overflow: auto;
  border-radius: 6px;
  background: #f5f7fa;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

@media (max-width: 720px) {
  .preview-stats {
    gap: 10px;
    flex-wrap: wrap;
  }

  .reading-surface {
    padding: 12px 0 24px;
  }

  .reading-surface :deep(h1) {
    font-size: 20px;
  }
}
</style>
