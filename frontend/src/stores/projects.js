import { defineStore } from 'pinia'
import { listProjects, createProject } from '../api/projects'

export const useProjectsStore = defineStore('projects', {
  state: () => ({
    items: [],
    loading: false,
    creating: false,
  }),
  actions: {
    async fetch() {
      this.loading = true
      try {
        this.items = await listProjects()
      } finally {
        this.loading = false
      }
    },
    async create(payload) {
      this.creating = true
      try {
        const created = await createProject(payload)
        this.items = [created, ...this.items]
        return created
      } finally {
        this.creating = false
      }
    },
  },
})
