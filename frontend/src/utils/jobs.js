export function latestJob(jobs) {
  if (!jobs?.length) return null
  return jobs[jobs.length - 1]
}

export function jobTypeLabel(jobType) {
  return {
    analyze: 'Analyze',
    generate: 'Generate',
    document_process: 'Document',
    document_reprocess: 'Document',
    knowledge_process: 'Knowledge',
    knowledge_reprocess: 'Knowledge',
  }[jobType] || jobType || '-'
}

export function jobStatusLabel(status) {
  return {
    pending: 'pending',
    running: 'running',
    retrying: 'retrying',
    failed: 'failed',
    succeeded: 'succeeded',
    canceled: 'canceled',
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
