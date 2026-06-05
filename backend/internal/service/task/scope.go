package task

import (
	"context"
	"sort"
	"strings"

	"caseagent/internal/db/models"
	retrievalservice "caseagent/internal/service/retrieval"
)

func inferAffectedKnowledge(requirements string, knowledgeEntries []models.KnowledgeBase) ([]string, []string) {
	requirements = normalizeMatchText(requirements)

	productSet := make(map[string]struct{})
	moduleSet := make(map[string]struct{})

	for _, entry := range knowledgeEntries {
		for _, candidate := range knowledgeCandidates(entry) {
			if candidate == "" {
				continue
			}
			if strings.Contains(requirements, normalizeMatchText(candidate)) {
				switch entry.Type {
				case "product":
					productSet[entry.Name] = struct{}{}
				case "module":
					moduleSet[entry.Name] = struct{}{}
				}
				break
			}
		}
	}

	products := mapKeys(productSet)
	modules := mapKeys(moduleSet)
	sort.Strings(products)
	sort.Strings(modules)

	return products, modules
}

func inferAffectedKnowledgeWithRetrieval(ctx context.Context, retriever Retriever, requirements string) ([]string, []string) {
	queries := buildKnowledgeQueries(requirements, nil, nil)
	results, err := retriever.SearchKnowledgeMultiQuery(ctx, queries, 6, "")
	if err != nil {
		return nil, nil
	}

	productSet := make(map[string]struct{})
	moduleSet := make(map[string]struct{})
	for _, result := range results {
		switch result.Type {
		case "product":
			productSet[result.Name] = struct{}{}
		case "module":
			moduleSet[result.Name] = struct{}{}
		}
	}

	products := mapKeys(productSet)
	modules := mapKeys(moduleSet)
	sort.Strings(products)
	sort.Strings(modules)
	return products, modules
}

func knowledgeCandidates(entry models.KnowledgeBase) []string {
	candidates := []string{entry.Name}
	for _, key := range []string{"aliases", "alias", "keywords"} {
		value, ok := entry.Metadata[key]
		if !ok {
			continue
		}

		switch typed := value.(type) {
		case string:
			candidates = append(candidates, typed)
		case []string:
			candidates = append(candidates, typed...)
		case []any:
			for _, item := range typed {
				if text, ok := item.(string); ok {
					candidates = append(candidates, text)
				}
			}
		}
	}

	return candidates
}

func formatKnowledgeContext(entries []models.KnowledgeBase) string {
	if len(entries) == 0 {
		return "未命中明确的知识库条目，请主要基于需求文档生成测试用例。"
	}

	var builder strings.Builder
	for _, entry := range entries {
		builder.WriteString("## ")
		builder.WriteString(entry.Type)
		builder.WriteString(": ")
		builder.WriteString(entry.Name)
		builder.WriteString("\n\n")
		builder.WriteString(strings.TrimSpace(entry.Content))
		builder.WriteString("\n\n")
	}

	return strings.TrimSpace(builder.String())
}

func retrieveKnowledgeFallback(ctx context.Context, retriever Retriever, requirements string, products []string, modules []string) []retrievalservice.KnowledgeResult {
	queries := buildKnowledgeQueries(requirements, products, modules)
	results, err := retriever.SearchKnowledgeMultiQuery(ctx, queries, 6, "")
	if err != nil {
		return nil
	}
	return results
}

func knowledgeResultsToBaseEntries(results []retrievalservice.KnowledgeResult) []models.KnowledgeBase {
	if len(results) == 0 {
		return nil
	}
	entries := make([]models.KnowledgeBase, 0, len(results))
	for _, result := range results {
		entries = append(entries, models.KnowledgeBase{
			ID:       result.ID,
			Type:     result.Type,
			Name:     result.Name,
			Content:  result.Content,
			Metadata: result.Metadata,
		})
	}
	return entries
}
