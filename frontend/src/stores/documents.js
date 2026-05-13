import { defineStore } from 'pinia'
import {
  listDocuments,
  uploadDocument,
  reprocessDocument,
  deleteDocument,
} from '../api/documents'

export const useDocumentsStore = defineStore('documents', {
  state: () => ({
    items: [],
    loading: false,
    uploading: false,
  }),
  actions: {
    async fetch(projectId) {
      this.loading = true
      try {
        this.items = await listDocuments(projectId)
      } finally {
        this.loading = false
      }
    },
    async upload(projectId, payload) {
      this.uploading = true
      try {
        const created = await uploadDocument(projectId, payload)
        this.items = [created, ...this.items]
        return created
      } finally {
        this.uploading = false
      }
    },
    async reprocess(id) {
      const updated = await reprocessDocument(id)
      this.replace(updated)
      return updated
    },
    async remove(id) {
      await deleteDocument(id)
      this.items = this.items.filter((d) => d.id !== id)
    },
    replace(updated) {
      const idx = this.items.findIndex((d) => d.id === updated.id)
      if (idx >= 0) this.items.splice(idx, 1, updated)
    },
  },
})
