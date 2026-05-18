import { defineStore } from 'pinia'
import {
  listKnowledgeSuggestions,
  updateKnowledgeSuggestion,
} from '../api/knowledgeSuggestions'

export const useKnowledgeSuggestionsStore = defineStore('knowledgeSuggestions', {
  state: () => ({
    items: [],
    loading: false,
    saving: false,
    statusFilter: 'pending',
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
    async setStatus(id, status) {
      this.saving = true
      try {
        const updated = await updateKnowledgeSuggestion(id, status)
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
  },
})
