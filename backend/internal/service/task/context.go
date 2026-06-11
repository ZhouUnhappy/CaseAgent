package task

import (
	"context"
	"fmt"
	"strings"

	"caseagent/internal/db/models"
	agentservice "caseagent/internal/service/agent"
	retrievalservice "caseagent/internal/service/retrieval"
)

// buildSourceContext assembles a JSONB-friendly map recording the retrieval
// context that fed this generation: queries used, document hits with rank /
// score / top chunks, knowledge hits with rank / score, and the union of
// knowledge IDs actually shipped to the agent (retrieval hits plus those
// matched via affected_products / affected_modules).
func buildSourceContext(documentQueries, knowledgeQueries []string,
	documentHits []retrievalservice.DocumentResult,
	knowledgeHits []retrievalservice.KnowledgeResult,
	knowledgeShipped []models.KnowledgeBase,
	modelCalls []agentservice.ModelCallTrace,
) map[string]any {
	docs := make([]map[string]any, 0, len(documentHits))
	for _, hit := range documentHits {
		chunks := make([]map[string]any, 0, len(hit.MatchedChunks))
		maxChunks := 3
		if len(hit.MatchedChunks) < maxChunks {
			maxChunks = len(hit.MatchedChunks)
		}
		for i := 0; i < maxChunks; i++ {
			c := hit.MatchedChunks[i]
			chunks = append(chunks, map[string]any{
				"text":  c.Text,
				"score": c.Score,
				"query": c.Query,
				"rank":  c.Rank,
			})
		}
		docs = append(docs, map[string]any{
			"document_id":   hit.DocumentID,
			"parent_doc_id": hit.ParentDocID,
			"name":          hit.Name,
			"rank":          hit.Rank,
			"best_score":    hit.BestScore,
			"hit_queries":   hit.HitQueries,
			"top_chunks":    chunks,
		})
	}

	kbHits := make([]map[string]any, 0, len(knowledgeHits))
	for _, hit := range knowledgeHits {
		kbHits = append(kbHits, map[string]any{
			"id":               hit.ID,
			"name":             hit.Name,
			"type":             hit.Type,
			"source":           hit.Source,
			"expires_at":       hit.ExpiresAt,
			"duplicate_of_id":  hit.DuplicateOfID,
			"source_highlight": hit.SourceHighlight,
			"rank":             hit.Rank,
			"score":            hit.Score,
			"hit_queries":      hit.HitQueries,
		})
	}

	shippedIDs := make([]int, 0, len(knowledgeShipped))
	shippedNames := make([]string, 0, len(knowledgeShipped))
	for _, entry := range knowledgeShipped {
		if entry.ID <= 0 {
			continue
		}
		shippedIDs = append(shippedIDs, entry.ID)
		shippedNames = append(shippedNames, entry.Name)
	}

	traceModelCalls := make([]map[string]any, 0, len(modelCalls))
	agentRuns := make([]map[string]any, 0, len(modelCalls))
	seenAgentRuns := make(map[int]struct{}, len(modelCalls))
	for _, call := range modelCalls {
		item := map[string]any{
			"id":                     call.ID,
			"agent_run_id":           call.AgentRunID,
			"agent":                  call.Agent,
			"attempt":                call.Attempt,
			"provider":               call.Provider,
			"model":                  call.Model,
			"provider_role":          call.ProviderRole,
			"status":                 call.Status,
			"prompt_id":              call.PromptID,
			"prompt_version":         call.PromptVersion,
			"prompt_chars":           call.PromptChars,
			"response_chars":         call.ResponseChars,
			"estimated_total_tokens": call.EstimatedTotalToken,
			"total_tokens":           call.TotalTokens,
			"token_source":           call.TokenSource,
			"last_error":             call.LastError,
		}
		traceModelCalls = append(traceModelCalls, item)
		if call.AgentRunID <= 0 {
			continue
		}
		if _, ok := seenAgentRuns[call.AgentRunID]; ok {
			continue
		}
		seenAgentRuns[call.AgentRunID] = struct{}{}
		agentRuns = append(agentRuns, map[string]any{
			"id":      call.AgentRunID,
			"agent":   call.Agent,
			"attempt": call.Attempt,
		})
	}

	return map[string]any{
		"document_queries":        documentQueries,
		"knowledge_queries":       knowledgeQueries,
		"document_hits":           docs,
		"knowledge_hits":          kbHits,
		"knowledge_shipped_ids":   shippedIDs,
		"knowledge_shipped_names": shippedNames,
		"agent_runs":              agentRuns,
		"model_calls":             traceModelCalls,
	}
}

func buildKnowledgeQueries(requirements string, products []string, modules []string) []string {
	queries := make([]string, 0, 1+len(products)+len(modules))
	if trimmed := strings.TrimSpace(requirements); trimmed != "" {
		queries = append(queries, trimmed)
	}
	queries = append(queries, splitRequirementFragments(requirements, 3)...)
	queries = append(queries, products...)
	queries = append(queries, modules...)
	if len(products) > 0 || len(modules) > 0 {
		queries = append(queries, strings.Join(append(append([]string{}, products...), modules...), " "))
	}
	return dedupeNonEmptyStrings(queries)
}

func splitRequirementFragments(requirements string, maxFragments int) []string {
	replacer := strings.NewReplacer(
		"\r", "\n",
		"。", "\n",
		"！", "\n",
		"？", "\n",
		";", "\n",
		"；", "\n",
		".", "\n",
		"!", "\n",
		"?", "\n",
	)

	segments := strings.Split(replacer.Replace(requirements), "\n")
	fragments := make([]string, 0, maxFragments)
	for _, segment := range segments {
		trimmed := strings.TrimSpace(segment)
		if len([]rune(trimmed)) < 8 {
			continue
		}
		fragments = append(fragments, trimmed)
		if len(fragments) >= maxFragments {
			break
		}
	}
	return fragments
}

func dedupeNonEmptyStrings(values []string) []string {
	deduped := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := normalizeMatchText(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		deduped = append(deduped, trimmed)
	}
	return deduped
}

func mergeKnowledgeEntries(primary []models.KnowledgeBase, secondary []models.KnowledgeBase) []models.KnowledgeBase {
	if len(primary) == 0 && len(secondary) == 0 {
		return nil
	}

	merged := make([]models.KnowledgeBase, 0, len(primary)+len(secondary))
	seen := make(map[int]struct{}, len(primary)+len(secondary))

	for _, entry := range primary {
		if entry.ID <= 0 {
			continue
		}
		if _, ok := seen[entry.ID]; ok {
			continue
		}
		seen[entry.ID] = struct{}{}
		merged = append(merged, entry)
	}

	for _, entry := range secondary {
		if entry.ID <= 0 {
			continue
		}
		if _, ok := seen[entry.ID]; ok {
			continue
		}
		seen[entry.ID] = struct{}{}
		merged = append(merged, entry)
	}

	return merged
}

func buildRequirementsContext(ctx context.Context, retriever Retriever, requirements string, documentIDs []int, products []string, modules []string) (string, []retrievalservice.DocumentResult) {
	queries := buildDocumentQueries(requirements, products, modules)
	if len(queries) == 0 || len(documentIDs) == 0 {
		return requirements, nil
	}

	results, err := retriever.SearchDocumentsMultiQuery(ctx, queries, 4, documentIDs)
	if err != nil || len(results) == 0 {
		return requirements, nil
	}

	var builder strings.Builder
	builder.WriteString("## 需求命中上下文（基于文档检索）\n\n")
	for _, result := range results {
		builder.WriteString(fmt.Sprintf("### [rank %d] %s (parent_doc_id=%d)\n\n", result.Rank, result.Name, result.ParentDocID))
		if len(result.HitQueries) > 0 {
			builder.WriteString("- 命中查询: ")
			builder.WriteString(strings.Join(result.HitQueries, " | "))
			builder.WriteString("\n")
		}
		builder.WriteString(fmt.Sprintf("- 综合得分: %.4f\n", result.BestScore))

		maxChunks := 3
		if len(result.MatchedChunks) < maxChunks {
			maxChunks = len(result.MatchedChunks)
		}
		if maxChunks > 0 {
			builder.WriteString("- 命中片段:\n")
			for idx := 0; idx < maxChunks; idx++ {
				chunk := result.MatchedChunks[idx]
				builder.WriteString(fmt.Sprintf("  %d. [score=%.4f, query=%q, chunk_rank=%d] %s\n",
					idx+1, chunk.Score, chunk.Query, chunk.Rank, chunk.Text))
			}
		}
		builder.WriteString("\n")
	}

	context := strings.TrimSpace(builder.String())
	if context == "" {
		return requirements, results
	}

	// If retrieval context is too short, append original requirements as fallback context.
	if len([]rune(context)) < 400 {
		return context + "\n\n## 完整需求文档\n\n" + requirements, results
	}
	return context, results
}

func buildDocumentQueries(requirements string, products []string, modules []string) []string {
	queries := make([]string, 0, 1+len(products)+len(modules))
	if trimmed := strings.TrimSpace(requirements); trimmed != "" {
		queries = append(queries, trimmed)
	}
	queries = append(queries, splitRequirementFragments(requirements, 4)...)
	queries = append(queries, products...)
	queries = append(queries, modules...)
	return dedupeNonEmptyStrings(queries)
}
