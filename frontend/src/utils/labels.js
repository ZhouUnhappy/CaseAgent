export function knowledgeTypeLabel(type) {
  return {
    product: '产品',
    module: '模块',
    context_gap: '上下文缺失',
  }[type] || type || '-'
}
