import { useDocumentsStore } from './documents'
import { useKnowledgeStore } from './knowledge'
import { useKnowledgeSuggestionsStore } from './knowledgeSuggestions'
import { useProjectsStore } from './projects'
import { useTasksStore } from './tasks'
import { useTestCasesStore } from './testcases'

export function clearTenantScopedStores() {
  useDocumentsStore().clear()
  useKnowledgeStore().clear()
  useKnowledgeSuggestionsStore().clear()
  useProjectsStore().clear()
  useTasksStore().clear()
  useTestCasesStore().clear()
}
