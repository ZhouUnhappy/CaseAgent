import { defineStore } from 'pinia'
import {
  listTasks,
  createTask,
  getTask,
  reviewAffected,
  generateCases,
  retryTask,
} from '../api/tasks'

export const useTasksStore = defineStore('tasks', {
  state: () => ({
    items: [],
    loading: false,
    creating: false,
    current: null,
  }),
  actions: {
    async fetch(projectId) {
      this.loading = true
      try {
        this.items = await listTasks(projectId)
      } finally {
        this.loading = false
      }
    },
    async create(projectId, payload) {
      this.creating = true
      try {
        const created = await createTask(projectId, payload)
        this.items = [created, ...this.items]
        return created
      } finally {
        this.creating = false
      }
    },
    async load(id) {
      this.current = await getTask(id)
      return this.current
    },
    async review(id, payload) {
      const updated = await reviewAffected(id, payload)
      this.replace(updated)
      this.current = updated
      return updated
    },
    async generate(id) {
      const updated = await generateCases(id)
      this.replace(updated)
      this.current = updated
      return updated
    },
    async retry(id) {
      const updated = await retryTask(id)
      this.replace(updated)
      this.current = updated
      return updated
    },
    replace(updated) {
      const idx = this.items.findIndex((t) => t.id === updated.id)
      if (idx >= 0) this.items.splice(idx, 1, updated)
    },
  },
})
