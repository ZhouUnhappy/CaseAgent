<script setup>
import { computed } from 'vue'

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

const blocks = computed(() => parseMarkdown(props.content || ''))

function cleanInline(value) {
  return String(value || '')
    .replace(/!\[([^\]]*)\]\([^)]+\)/g, '$1')
    .replace(/\[([^\]]+)\]\(([^)]+)\)/g, '$1 ($2)')
    .replace(/\*\*([^*]+)\*\*/g, '$1')
    .replace(/__([^_]+)__/g, '$1')
    .replace(/`([^`]+)`/g, '$1')
    .trim()
}

// Render a safe reading subset without injecting document HTML into the page.
function parseMarkdown(content) {
  const lines = String(content || '').replace(/\r\n/g, '\n').split('\n')
  const result = []
  let paragraph = []
  let list = null
  let code = null

  const flushParagraph = () => {
    const text = cleanInline(paragraph.join(' '))
    if (text) result.push({ type: 'paragraph', text })
    paragraph = []
  }

  const flushList = () => {
    if (list?.items.length) result.push(list)
    list = null
  }

  const flushCode = () => {
    if (code) result.push(code)
    code = null
  }

  for (const line of lines) {
    const trimmed = line.trim()

    if (code) {
      if (/^```/.test(trimmed)) {
        flushCode()
      } else {
        code.text += `${code.text ? '\n' : ''}${line}`
      }
      continue
    }

    const fence = trimmed.match(/^```\s*([^\s]*)/)
    if (fence) {
      flushParagraph()
      flushList()
      code = { type: 'code', language: fence[1] || '', text: '' }
      continue
    }

    if (!trimmed) {
      flushParagraph()
      flushList()
      continue
    }

    const heading = trimmed.match(/^(#{1,4})\s+(.+)$/)
    if (heading) {
      flushParagraph()
      flushList()
      result.push({ type: 'heading', level: heading[1].length, text: cleanInline(heading[2]) })
      continue
    }

    if (/^(---+|___+|\*\*\*+)$/.test(trimmed)) {
      flushParagraph()
      flushList()
      result.push({ type: 'divider' })
      continue
    }

    const unordered = trimmed.match(/^[-*+]\s+(.+)$/)
    const ordered = trimmed.match(/^\d+[.)]\s+(.+)$/)
    if (unordered || ordered) {
      flushParagraph()
      const listType = ordered ? 'ordered-list' : 'unordered-list'
      if (list?.type !== listType) {
        flushList()
        list = { type: listType, items: [] }
      }
      list.items.push(cleanInline((unordered || ordered)[1]))
      continue
    }

    const quote = trimmed.match(/^>\s?(.*)$/)
    if (quote) {
      flushParagraph()
      flushList()
      result.push({ type: 'quote', text: cleanInline(quote[1]) })
      continue
    }

    flushList()
    paragraph.push(trimmed)
  }

  flushParagraph()
  flushList()
  flushCode()
  return result
}
</script>

<template>
  <div class="markdown-preview">
    <div class="preview-stats">
      <span>{{ stats.chars.toLocaleString() }} 字符</span>
      <span>{{ stats.lines.toLocaleString() }} 行</span>
      <span>{{ stats.headings.toLocaleString() }} 个标题</span>
    </div>

    <div v-if="blocks.length" class="reading-surface">
      <template v-for="(block, index) in blocks" :key="`${block.type}-${index}`">
        <component
          :is="`h${Math.min(block.level + 1, 5)}`"
          v-if="block.type === 'heading'"
          class="document-heading"
        >{{ block.text }}</component>
        <p v-else-if="block.type === 'paragraph'" class="document-paragraph">{{ block.text }}</p>
        <ul v-else-if="block.type === 'unordered-list'" class="document-list">
          <li v-for="(item, itemIndex) in block.items" :key="itemIndex">{{ item }}</li>
        </ul>
        <ol v-else-if="block.type === 'ordered-list'" class="document-list">
          <li v-for="(item, itemIndex) in block.items" :key="itemIndex">{{ item }}</li>
        </ol>
        <blockquote v-else-if="block.type === 'quote'" class="document-quote">{{ block.text }}</blockquote>
        <pre v-else-if="block.type === 'code'" class="document-code"><code>{{ block.text }}</code></pre>
        <el-divider v-else-if="block.type === 'divider'" />
      </template>
    </div>
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

.document-heading {
  margin: 26px 0 10px;
  color: #1f2329;
  letter-spacing: 0;
}

h2.document-heading {
  font-size: 22px;
}

h3.document-heading {
  font-size: 18px;
}

h4.document-heading,
h5.document-heading {
  font-size: 16px;
}

.document-paragraph {
  margin: 10px 0;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.document-list {
  margin: 10px 0;
  padding-left: 26px;
}

.document-list li + li {
  margin-top: 6px;
}

.document-quote {
  margin: 14px 0;
  padding: 10px 14px;
  border-left: 3px solid #409eff;
  background: #f5f7fa;
  color: #606266;
}

.document-code {
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

  h2.document-heading {
    font-size: 20px;
  }
}
</style>
