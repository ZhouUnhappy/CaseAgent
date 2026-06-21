export function latestJob(jobs) {
  if (!jobs?.length) return null
  return jobs[jobs.length - 1]
}

export function jobTypeLabel(jobType) {
  return {
    analyze: '影响范围分析',
    generate: '用例生成',
    document_process: '需求文档处理',
    document_reprocess: '需求文档重处理',
    knowledge_process: '知识处理',
    knowledge_reprocess: '知识重处理',
  }[jobType] || jobType || '-'
}

export function jobStatusLabel(status) {
  return {
    pending: '等待中',
    running: '运行中',
    retrying: '重试中',
    failed: '失败',
    succeeded: '已完成',
    canceled: '已取消',
  }[status] || status || '-'
}

export function jobStatusType(status) {
  return {
    pending: 'info',
    running: 'warning',
    retrying: 'warning',
    failed: 'danger',
    succeeded: 'success',
    canceled: 'info',
  }[status] || 'info'
}

export function compactJobError(job) {
  const message = job?.last_error || ''
  if (!message) return ''
  return message.length > 120 ? `${message.slice(0, 117)}...` : message
}
