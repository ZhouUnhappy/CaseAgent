const PREFIX = 'caseagent.knowledge_suggestion_draft.'

export function suggestionDraftStorageKey(id) {
  return `${PREFIX}${id}`
}

export function writeSuggestionDraft(id, content) {
  sessionStorage.setItem(suggestionDraftStorageKey(id), content ?? '')
}

export function readAndClearSuggestionDraft(id) {
  if (!id) return ''
  const key = suggestionDraftStorageKey(id)
  const value = sessionStorage.getItem(key) ?? ''
  sessionStorage.removeItem(key)
  return value
}
