import { defineStore } from 'pinia'
import {
  listTestCases,
  updateTestCase,
  submitTestCase,
  batchUpdateTestCases,
  batchSubmitTestCases,
  createCaseFeedback,
  createBatchCaseFeedback,
} from '../api/testcases'

export const useTestCasesStore = defineStore('testcases', {
  state: () => ({
    items: [],
    loading: false,
    saving: false,
    feedbackSaving: false,
    batchSaving: false,
  }),
  actions: {
    async fetch(taskId) {
      this.loading = true
      try {
        this.items = await listTestCases(taskId)
      } finally {
        this.loading = false
      }
    },
    async update(taskId, caseId, payload) {
      this.saving = true
      try {
        const updated = await updateTestCase(taskId, caseId, payload)
        this.replace(updated)
        return updated
      } finally {
        this.saving = false
      }
    },
    async submit(taskId, caseId) {
      const updated = await submitTestCase(taskId, caseId)
      this.replace(updated)
      return updated
    },
    async batchUpdate(taskId, payload) {
      this.batchSaving = true
      try {
        const updatedRows = await batchUpdateTestCases(taskId, payload)
        updatedRows.forEach((row) => this.replace(row))
        return updatedRows
      } finally {
        this.batchSaving = false
      }
    },
    async batchSubmit(taskId, testCaseIds) {
      this.batchSaving = true
      try {
        const updatedRows = await batchSubmitTestCases(taskId, { test_case_ids: testCaseIds })
        updatedRows.forEach((row) => this.replace(row))
        return updatedRows
      } finally {
        this.batchSaving = false
      }
    },
    async feedback(taskId, caseId, payload) {
      this.feedbackSaving = true
      try {
        return await createCaseFeedback(taskId, caseId, payload)
      } finally {
        this.feedbackSaving = false
      }
    },
    async batchFeedback(taskId, payload) {
      this.feedbackSaving = true
      try {
        return await createBatchCaseFeedback(taskId, payload)
      } finally {
        this.feedbackSaving = false
      }
    },
    clear() {
      this.items = []
      this.loading = false
      this.saving = false
      this.feedbackSaving = false
      this.batchSaving = false
    },
    replace(updated) {
      const idx = this.items.findIndex((c) => c.id === updated.id)
      if (idx >= 0) this.items.splice(idx, 1, updated)
    },
  },
})
