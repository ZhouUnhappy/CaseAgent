import { defineStore } from 'pinia'
import {
  createKnowledgeSuggestion,
  draftKnowledgeSuggestion,
  listKnowledgeSuggestions,
  updateKnowledgeSuggestion,
} from '../api/knowledgeSuggestions'

export const useKnowledgeSuggestionsStore = defineStore('knowledgeSuggestions', {
  state: () => ({
    items: [],
    loading: false,
    saving: false,
    draftingId: null,
    statusFilter: 'pending',
    showAutoExpired: false,
  }),
  actions: {
    async fetch() {
      this.loading = true
      try {
        this.items = (await listKnowledgeSuggestions(this.statusFilter || undefined)) ?? []
      } finally {
        this.loading = false
      }
    },
    setStatusFilter(value) {
      this.statusFilter = value
      return this.fetch()
    },
    setShowAutoExpired(value) {
      this.showAutoExpired = value
    },
    async createManual(payload) {
      this.saving = true
      try {
        const created = await createKnowledgeSuggestion(payload)
        const idx = this.items.findIndex((s) => s.id === created.id)
        if (idx >= 0) {
          this.items.splice(idx, 1, created)
        } else if (!this.statusFilter || created.status === this.statusFilter) {
          this.items = [created, ...this.items]
        }
        return created
      } finally {
        this.saving = false
      }
    },
    async draft(id) {
      this.draftingId = id
      try {
        return await draftKnowledgeSuggestion(id)
      } finally {
        this.draftingId = null
      }
    },
    async setStatus(id, status, resolvedKnowledgeId) {
      this.saving = true
      try {
        const updated = await updateKnowledgeSuggestion(id, status, resolvedKnowledgeId)
        const idx = this.items.findIndex((s) => s.id === updated.id)
        if (idx >= 0) {
          // 若当前过滤为 pending，状态变更后这行应该消失
          if (this.statusFilter && updated.status !== this.statusFilter) {
            this.items.splice(idx, 1)
          } else {
            this.items.splice(idx, 1, updated)
          }
        }
        return updated
      } finally {
        this.saving = false
      }
    },
    clear() {
      this.items = []
      this.loading = false
      this.saving = false
      this.draftingId = null
    },
  },
})
