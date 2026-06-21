const STATUS_META = {
  pending: { type: 'info', label: '待处理' },
  processing: { type: 'warning', label: '处理中' },
  completed: { type: 'success', label: '已完成' },
  failed: { type: 'danger', label: '失败' },
  canceled: { type: 'info', label: '已取消' },

  analyzing: { type: 'warning', label: '分析中' },
  awaiting_review: { type: 'info', label: '待确认范围' },
  ready_to_generate: { type: 'info', label: '待生成' },
  generating: { type: 'warning', label: '生成中' },

  draft: { type: 'info', label: '草稿' },
  submitted: { type: 'success', label: '已提交' },
  approved: { type: 'success', label: '已通过' },

  adopted: { type: 'success', label: '已采纳' },
  dismissed: { type: 'info', label: '已忽略' },
}

export function statusMeta(status) {
  return STATUS_META[status] || { type: '', label: status || '-' }
}

export function statusLabel(status) {
  return statusMeta(status).label
}
