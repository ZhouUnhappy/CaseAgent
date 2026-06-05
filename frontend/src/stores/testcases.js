import { defineStore } from 'pinia'
import {
  listTestCases,
  updateTestCase,
  submitTestCase,
} from '../api/testcases'

export const useTestCasesStore = defineStore('testcases', {
  state: () => ({
    items: [],
    loading: false,
    saving: false,
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
    clear() {
      this.items = []
    },
    replace(updated) {
      const idx = this.items.findIndex((c) => c.id === updated.id)
      if (idx >= 0) this.items.splice(idx, 1, updated)
    },
  },
})
