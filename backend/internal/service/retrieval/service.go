package retrieval

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"caseagent/internal/config"
	"caseagent/internal/db/models"
	"caseagent/internal/db/pgvector"

	"github.com/uptrace/bun"
)

const defaultTopK = 5

type Service struct {
	db *bun.DB
}

type DocumentResult struct {
	DocumentID    int      `json:"document_id"`
	ParentDocID   int      `json:"parent_doc_id"`
	Name          string   `json:"name"`
	MatchedChunks []string `json:"matched_chunks"`
	Content       string   `json:"content"`
}

type KnowledgeResult struct {
	ID       int            `json:"id"`
	Type     string         `json:"type"`
	Name     string         `json:"name"`
	Content  string         `json:"content"`
	Metadata map[string]any `json:"metadata"`
}

func New(db *bun.DB) *Service {
	return &Service{db: db}
}

func (s *Service) SearchDocuments(ctx context.Context, query string, topK int, documentIDs []int) ([]DocumentResult, error) {
	retriever, err := s.newRetriever(ctx)
	if err != nil {
		return nil, err
	}

	rawChunks, err := retriever.RetrieveWithQuery(ctx, query, retrievalPoolSize(topK))
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve document chunks: %w", err)
	}

	documentFilter := make(map[int]struct{}, len(documentIDs))
	for _, id := range documentIDs {
		documentFilter[id] = struct{}{}
	}

	parentIDs := make([]int, 0, len(rawChunks))
	parentSeen := make(map[int]struct{}, len(rawChunks))
	matchedChunks := make(map[int][]string, len(rawChunks))

	for _, chunk := range rawChunks {
		parentID := chunk.ParentDocID
		if parentID == 0 {
			parentID = chunk.DocumentID
		}

		if len(documentFilter) > 0 {
			if _, ok := documentFilter[parentID]; !ok {
				continue
			}
		}

		if _, ok := parentSeen[parentID]; !ok {
			parentSeen[parentID] = struct{}{}
			parentIDs = append(parentIDs, parentID)
		}
		matchedChunks[parentID] = append(matchedChunks[parentID], strings.TrimSpace(chunk.Content))
	}

	if len(parentIDs) == 0 {
		return []DocumentResult{}, nil
	}

	if topK <= 0 {
		topK = defaultTopK
	}
	if len(parentIDs) > topK {
		parentIDs = parentIDs[:topK]
	}

	documents, err := s.loadDocumentsByID(ctx, parentIDs)
	if err != nil {
		return nil, err
	}
	parentIDs = filterSearchableDocumentIDs(parentIDs, documents)
	if len(parentIDs) == 0 {
		return []DocumentResult{}, nil
	}

	contents, err := s.loadDocumentContents(ctx, parentIDs)
	if err != nil {
		return nil, err
	}

	results := make([]DocumentResult, 0, len(parentIDs))
	for _, parentID := range parentIDs {
		document := documents[parentID]
		results = append(results, DocumentResult{
			DocumentID:    parentID,
			ParentDocID:   parentID,
			Name:          document.Name,
			MatchedChunks: matchedChunks[parentID],
			Content:       preferredDocumentContent(document.Content, contents[parentID]),
		})
	}

	return results, nil
}

func (s *Service) SearchDocumentsMultiQuery(ctx context.Context, queries []string, topK int, documentIDs []int) ([]DocumentResult, error) {
	normalizedQueries := normalizeQueries(queries)
	if len(normalizedQueries) == 0 {
		return []DocumentResult{}, nil
	}

	if topK <= 0 {
		topK = defaultTopK
	}

	resultSets := make([][]DocumentResult, 0, len(normalizedQueries))
	for _, query := range normalizedQueries {
		results, err := s.SearchDocuments(ctx, query, topK, documentIDs)
		if err != nil {
			return nil, fmt.Errorf("search documents with query %q: %w", query, err)
		}
		resultSets = append(resultSets, results)
	}

	return mergeDocumentResultSets(resultSets, topK), nil
}

func (s *Service) SearchKnowledge(ctx context.Context, query string, topK int, kbType string) ([]KnowledgeResult, error) {
	retriever, err := s.newRetriever(ctx)
	if err != nil {
		return nil, err
	}

	rawEntries, err := retriever.RetrieveKnowledgeWithQuery(ctx, query, retrievalPoolSize(topK))
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve knowledge entries: %w", err)
	}

	if topK <= 0 {
		topK = defaultTopK
	}

	results := make([]KnowledgeResult, 0, topK)
	seen := make(map[int]struct{}, len(rawEntries))
	for _, entry := range rawEntries {
		if entry == nil {
			continue
		}
		if !isKnowledgeSearchable(entry) {
			continue
		}
		if kbType != "" && entry.Type != kbType {
			continue
		}
		if _, ok := seen[entry.ID]; ok {
			continue
		}
		seen[entry.ID] = struct{}{}
		results = append(results, KnowledgeResult{
			ID:       entry.ID,
			Type:     entry.Type,
			Name:     entry.Name,
			Content:  entry.Content,
			Metadata: entry.Metadata,
		})
		if len(results) >= topK {
			break
		}
	}

	return results, nil
}

func (s *Service) SearchKnowledgeMultiQuery(ctx context.Context, queries []string, topK int, kbType string) ([]KnowledgeResult, error) {
	normalizedQueries := normalizeQueries(queries)
	if len(normalizedQueries) == 0 {
		return []KnowledgeResult{}, nil
	}

	if topK <= 0 {
		topK = defaultTopK
	}

	resultSets := make([][]KnowledgeResult, 0, len(normalizedQueries))
	for _, query := range normalizedQueries {
		results, err := s.SearchKnowledge(ctx, query, topK, kbType)
		if err != nil {
			return nil, fmt.Errorf("search knowledge with query %q: %w", query, err)
		}
		resultSets = append(resultSets, results)
	}

	return mergeKnowledgeResultSets(resultSets, topK), nil
}

func (s *Service) newRetriever(ctx context.Context) (*pgvector.Retriever, error) {
	cfg := config.Get()
	return pgvector.NewRetriever(ctx, &pgvector.RetrieverConfig{
		Provider:   cfg.Model.Embedding.Provider,
		DB:         s.db,
		Dimensions: cfg.Model.Embedding.Dimensions,
		APIKey:     cfg.Model.Embedding.APIKey,
		AccessKey:  cfg.Model.Embedding.AccessKey,
		SecretKey:  cfg.Model.Embedding.SecretKey,
		BaseURL:    cfg.Model.Embedding.BaseURL,
		Region:     cfg.Model.Embedding.Region,
		Model:      cfg.Model.Embedding.Model,
	})
}

func (s *Service) loadDocumentsByID(ctx context.Context, documentIDs []int) (map[int]models.Document, error) {
	var documents []models.Document
	if err := s.db.NewSelect().
		Model(&documents).
		Where("id IN (?)", bun.In(documentIDs)).
		Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to load documents: %w", err)
	}

	docMap := make(map[int]models.Document, len(documents))
	for _, document := range documents {
		docMap[document.ID] = document
	}

	return docMap, nil
}

func (s *Service) loadDocumentContents(ctx context.Context, documentIDs []int) (map[int]string, error) {
	var chunks []models.DocumentChunk
	if err := s.db.NewSelect().
		Model(&chunks).
		Where("document_id IN (?)", bun.In(documentIDs)).
		OrderExpr("document_id ASC, id ASC").
		Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to load document chunks: %w", err)
	}

	builder := make(map[int]*strings.Builder, len(documentIDs))
	for _, id := range documentIDs {
		builder[id] = &strings.Builder{}
	}

	for _, chunk := range chunks {
		buf, ok := builder[chunk.DocumentID]
		if !ok {
			continue
		}
		content := strings.TrimSpace(chunk.Content)
		if content == "" {
			continue
		}
		if buf.Len() > 0 {
			buf.WriteString("\n\n")
		}
		buf.WriteString(content)
	}

	contents := make(map[int]string, len(builder))
	for id, buf := range builder {
		contents[id] = buf.String()
	}

	return contents, nil
}

func retrievalPoolSize(topK int) int {
	if topK <= 0 {
		return defaultTopK * 3
	}
	return topK * 3
}

func preferredDocumentContent(stored string, fallback string) string {
	stored = strings.TrimSpace(stored)
	if stored != "" {
		return stored
	}
	return fallback
}

func filterSearchableDocumentIDs(documentIDs []int, documents map[int]models.Document) []int {
	filtered := make([]int, 0, len(documentIDs))
	for _, id := range documentIDs {
		document, ok := documents[id]
		if !ok || !isDocumentSearchable(document) {
			continue
		}
		filtered = append(filtered, id)
	}
	return filtered
}

func isDocumentSearchable(document models.Document) bool {
	return document.Status == models.DocumentStatusCompleted
}

func isKnowledgeSearchable(entry *models.KnowledgeBase) bool {
	return entry != nil && entry.Status == models.KnowledgeStatusCompleted
}

func normalizeQueries(queries []string) []string {
	normalized := make([]string, 0, len(queries))
	seen := make(map[string]struct{}, len(queries))
	for _, query := range queries {
		trimmed := strings.TrimSpace(query)
		if trimmed == "" {
			continue
		}

		key := strings.ToLower(strings.Join(strings.Fields(trimmed), " "))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	return normalized
}

func mergeKnowledgeResultSets(resultSets [][]KnowledgeResult, topK int) []KnowledgeResult {
	if topK <= 0 {
		topK = defaultTopK
	}

	type scoredResult struct {
		result   KnowledgeResult
		score    float64
		bestRank int
		firstSet int
	}

	scoredByID := make(map[int]*scoredResult)
	for setIdx, set := range resultSets {
		for rank, item := range set {
			score := 1.0 / float64(rank+1)
			entry, ok := scoredByID[item.ID]
			if !ok {
				scoredByID[item.ID] = &scoredResult{
					result:   item,
					score:    score,
					bestRank: rank,
					firstSet: setIdx,
				}
				continue
			}

			entry.score += score
			if rank < entry.bestRank {
				entry.bestRank = rank
				entry.result = item
			}
		}
	}

	scored := make([]scoredResult, 0, len(scoredByID))
	for _, item := range scoredByID {
		scored = append(scored, *item)
	}

	sort.SliceStable(scored, func(i int, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		if scored[i].bestRank != scored[j].bestRank {
			return scored[i].bestRank < scored[j].bestRank
		}
		if scored[i].firstSet != scored[j].firstSet {
			return scored[i].firstSet < scored[j].firstSet
		}
		return scored[i].result.ID < scored[j].result.ID
	})

	if len(scored) > topK {
		scored = scored[:topK]
	}

	results := make([]KnowledgeResult, 0, len(scored))
	for _, item := range scored {
		results = append(results, item.result)
	}

	return results
}

func mergeDocumentResultSets(resultSets [][]DocumentResult, topK int) []DocumentResult {
	if topK <= 0 {
		topK = defaultTopK
	}

	type scoredResult struct {
		result   DocumentResult
		score    float64
		bestRank int
		firstSet int
	}

	scoredByID := make(map[int]*scoredResult)
	for setIdx, set := range resultSets {
		for rank, item := range set {
			score := 1.0 / float64(rank+1)
			entry, ok := scoredByID[item.DocumentID]
			if !ok {
				normalized := item
				normalized.MatchedChunks = dedupeChunks(item.MatchedChunks)
				scoredByID[item.DocumentID] = &scoredResult{
					result:   normalized,
					score:    score,
					bestRank: rank,
					firstSet: setIdx,
				}
				continue
			}

			entry.score += score
			entry.result.MatchedChunks = dedupeChunks(append(entry.result.MatchedChunks, item.MatchedChunks...))
			if rank < entry.bestRank {
				entry.bestRank = rank
				entry.result.ParentDocID = item.ParentDocID
				entry.result.Name = item.Name
				entry.result.Content = item.Content
			}
		}
	}

	scored := make([]scoredResult, 0, len(scoredByID))
	for _, item := range scoredByID {
		scored = append(scored, *item)
	}

	sort.SliceStable(scored, func(i int, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		if scored[i].bestRank != scored[j].bestRank {
			return scored[i].bestRank < scored[j].bestRank
		}
		if scored[i].firstSet != scored[j].firstSet {
			return scored[i].firstSet < scored[j].firstSet
		}
		return scored[i].result.DocumentID < scored[j].result.DocumentID
	})

	if len(scored) > topK {
		scored = scored[:topK]
	}

	results := make([]DocumentResult, 0, len(scored))
	for _, item := range scored {
		results = append(results, item.result)
	}
	return results
}

func dedupeChunks(chunks []string) []string {
	deduped := make([]string, 0, len(chunks))
	seen := make(map[string]struct{}, len(chunks))
	for _, chunk := range chunks {
		trimmed := strings.TrimSpace(chunk)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(strings.Join(strings.Fields(trimmed), " "))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		deduped = append(deduped, trimmed)
	}
	return deduped
}
