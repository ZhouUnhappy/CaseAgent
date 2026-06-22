package retrieval

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"caseagent/internal/config"
	"caseagent/internal/db/models"
	"caseagent/internal/db/pgvector"

	"github.com/uptrace/bun"
)

const defaultTopK = 5

type Service struct {
	db bun.IDB
}

// MatchedChunk records a single chunk that contributed to a document hit, along
// with the query that produced it, its rank within that query, and the cosine
// similarity score returned by pgvector.
type MatchedChunk struct {
	Text  string  `json:"text"`
	Score float64 `json:"score"`
	Query string  `json:"query"`
	Rank  int     `json:"rank"`
}

type DocumentResult struct {
	DocumentID    int            `json:"document_id"`
	ParentDocID   int            `json:"parent_doc_id"`
	Name          string         `json:"name"`
	MatchedChunks []MatchedChunk `json:"matched_chunks"`
	HitQueries    []string       `json:"hit_queries"`
	BestScore     float64        `json:"best_score"`
	Rank          int            `json:"rank"`
	Content       string         `json:"content"`
}

type KnowledgeResult struct {
	ID              int                      `json:"id"`
	Type            string                   `json:"type"`
	Name            string                   `json:"name"`
	Content         string                   `json:"content"`
	Metadata        map[string]any           `json:"metadata"`
	Source          string                   `json:"source"`
	ExpiresAt       *time.Time               `json:"expires_at,omitempty"`
	DuplicateOfID   *int                     `json:"duplicate_of_id,omitempty"`
	SourceHighlight KnowledgeSourceHighlight `json:"source_highlight"`
	Score           float64                  `json:"score"`
	HitQueries      []string                 `json:"hit_queries"`
	Rank            int                      `json:"rank"`
}

type KnowledgeSourceHighlight struct {
	Source        string     `json:"source"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	DuplicateOfID *int       `json:"duplicate_of_id,omitempty"`
	IsExpired     bool       `json:"is_expired"`
	IsDuplicate   bool       `json:"is_duplicate"`
}

func New(db bun.IDB) *Service {
	return &Service{db: db}
}

func (s *Service) SearchDocuments(ctx context.Context, query string, topK int, documentIDs []int) ([]DocumentResult, error) {
	retriever, err := s.newRetriever(ctx)
	if err != nil {
		return nil, err
	}

	hits, err := retriever.RetrieveWithQuery(ctx, query, retrievalPoolSize(topK))
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve document chunks: %w", err)
	}

	documentFilter := make(map[int]struct{}, len(documentIDs))
	for _, id := range documentIDs {
		documentFilter[id] = struct{}{}
	}

	parentIDs := make([]int, 0, len(hits))
	parentSeen := make(map[int]struct{}, len(hits))
	matchedChunks := make(map[int][]MatchedChunk, len(hits))
	bestScore := make(map[int]float64, len(hits))

	for chunkIdx, hit := range hits {
		if hit.Chunk == nil {
			continue
		}
		parentID := hit.Chunk.ParentDocID
		if parentID == 0 {
			parentID = hit.Chunk.DocumentID
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
		matchedChunks[parentID] = append(matchedChunks[parentID], MatchedChunk{
			Text:  strings.TrimSpace(hit.Chunk.Content),
			Score: hit.Score,
			Query: query,
			Rank:  chunkIdx + 1,
		})
		if hit.Score > bestScore[parentID] {
			bestScore[parentID] = hit.Score
		}
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

	results := make([]DocumentResult, 0, len(parentIDs))
	for idx, parentID := range parentIDs {
		document := documents[parentID]
		results = append(results, DocumentResult{
			DocumentID:    parentID,
			ParentDocID:   parentID,
			Name:          document.Name,
			MatchedChunks: matchedChunks[parentID],
			HitQueries:    []string{query},
			BestScore:     bestScore[parentID],
			Rank:          idx + 1,
			Content:       strings.TrimSpace(document.Content),
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

	hits, err := retriever.RetrieveKnowledgeWithQuery(ctx, query, retrievalPoolSize(topK))
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve knowledge entries: %w", err)
	}

	if topK <= 0 {
		topK = defaultTopK
	}

	results := make([]KnowledgeResult, 0, topK)
	seen := make(map[int]struct{}, len(hits))
	now := time.Now()
	for _, hit := range hits {
		entry := hit.Knowledge
		if entry == nil {
			continue
		}
		if !isKnowledgeSearchableAt(entry, now) {
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
			ID:              entry.ID,
			Type:            entry.Type,
			Name:            entry.Name,
			Content:         entry.Content,
			Metadata:        entry.Metadata,
			Source:          knowledgeSource(entry),
			ExpiresAt:       entry.ExpiresAt,
			DuplicateOfID:   entry.DuplicateOfID,
			SourceHighlight: knowledgeSourceHighlight(entry, now),
			Score:           hit.Score,
			HitQueries:      []string{query},
			Rank:            len(results) + 1,
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

func retrievalPoolSize(topK int) int {
	if topK <= 0 {
		return defaultTopK * 3
	}
	return topK * 3
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
	return isKnowledgeSearchableAt(entry, time.Now())
}

func isKnowledgeSearchableAt(entry *models.KnowledgeBase, now time.Time) bool {
	if entry == nil || entry.Status != models.KnowledgeStatusCompleted {
		return false
	}
	if entry.DuplicateOfID != nil {
		return false
	}
	return !isKnowledgeExpiredAt(entry, now)
}

func isKnowledgeExpiredAt(entry *models.KnowledgeBase, now time.Time) bool {
	return entry != nil && entry.ExpiresAt != nil && !entry.ExpiresAt.After(now)
}

func knowledgeSource(entry *models.KnowledgeBase) string {
	if entry == nil || strings.TrimSpace(entry.Source) == "" {
		return "manual"
	}
	return entry.Source
}

func knowledgeSourceHighlight(entry *models.KnowledgeBase, now time.Time) KnowledgeSourceHighlight {
	if entry == nil {
		return KnowledgeSourceHighlight{Source: "manual"}
	}
	return KnowledgeSourceHighlight{
		Source:        knowledgeSource(entry),
		ExpiresAt:     entry.ExpiresAt,
		DuplicateOfID: entry.DuplicateOfID,
		IsExpired:     isKnowledgeExpiredAt(entry, now),
		IsDuplicate:   entry.DuplicateOfID != nil,
	}
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
		result    KnowledgeResult
		mergeRank float64
		bestRank  int
		firstSet  int
	}

	scoredByID := make(map[int]*scoredResult)
	for setIdx, set := range resultSets {
		for rank, item := range set {
			rankScore := 1.0 / float64(rank+1)
			entry, ok := scoredByID[item.ID]
			if !ok {
				normalized := item
				normalized.HitQueries = dedupeStrings(item.HitQueries)
				scoredByID[item.ID] = &scoredResult{
					result:    normalized,
					mergeRank: rankScore,
					bestRank:  rank,
					firstSet:  setIdx,
				}
				continue
			}

			entry.mergeRank += rankScore
			entry.result.HitQueries = dedupeStrings(append(entry.result.HitQueries, item.HitQueries...))
			if item.Score > entry.result.Score {
				entry.result.Score = item.Score
			}
			if rank < entry.bestRank {
				entry.bestRank = rank
			}
		}
	}

	scored := make([]scoredResult, 0, len(scoredByID))
	for _, item := range scoredByID {
		scored = append(scored, *item)
	}

	sort.SliceStable(scored, func(i int, j int) bool {
		if scored[i].mergeRank != scored[j].mergeRank {
			return scored[i].mergeRank > scored[j].mergeRank
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
	for idx, item := range scored {
		r := item.result
		r.Rank = idx + 1
		results = append(results, r)
	}

	return results
}

func mergeDocumentResultSets(resultSets [][]DocumentResult, topK int) []DocumentResult {
	if topK <= 0 {
		topK = defaultTopK
	}

	type scoredResult struct {
		result    DocumentResult
		mergeRank float64
		bestRank  int
		firstSet  int
	}

	scoredByID := make(map[int]*scoredResult)
	for setIdx, set := range resultSets {
		for rank, item := range set {
			rankScore := 1.0 / float64(rank+1)
			entry, ok := scoredByID[item.DocumentID]
			if !ok {
				normalized := item
				normalized.MatchedChunks = dedupeMatchedChunks(item.MatchedChunks)
				normalized.HitQueries = dedupeStrings(item.HitQueries)
				scoredByID[item.DocumentID] = &scoredResult{
					result:    normalized,
					mergeRank: rankScore,
					bestRank:  rank,
					firstSet:  setIdx,
				}
				continue
			}

			entry.mergeRank += rankScore
			entry.result.MatchedChunks = dedupeMatchedChunks(append(entry.result.MatchedChunks, item.MatchedChunks...))
			entry.result.HitQueries = dedupeStrings(append(entry.result.HitQueries, item.HitQueries...))
			if item.BestScore > entry.result.BestScore {
				entry.result.BestScore = item.BestScore
			}
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
		if scored[i].mergeRank != scored[j].mergeRank {
			return scored[i].mergeRank > scored[j].mergeRank
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
	for idx, item := range scored {
		r := item.result
		r.Rank = idx + 1
		results = append(results, r)
	}
	return results
}

// dedupeMatchedChunks collapses chunks with identical normalized text, keeping
// the entry with the highest score (and accumulating its query/rank from the
// first occurrence in the merged list).
func dedupeMatchedChunks(chunks []MatchedChunk) []MatchedChunk {
	deduped := make([]MatchedChunk, 0, len(chunks))
	bestIdx := make(map[string]int, len(chunks))
	for _, chunk := range chunks {
		trimmed := strings.TrimSpace(chunk.Text)
		if trimmed == "" {
			continue
		}
		chunk.Text = trimmed
		key := strings.ToLower(strings.Join(strings.Fields(trimmed), " "))
		if idx, ok := bestIdx[key]; ok {
			if chunk.Score > deduped[idx].Score {
				deduped[idx] = chunk
			}
			continue
		}
		bestIdx[key] = len(deduped)
		deduped = append(deduped, chunk)
	}
	// Keep deduped chunks ordered by score desc, then original rank asc for stable output.
	sort.SliceStable(deduped, func(i int, j int) bool {
		if deduped[i].Score != deduped[j].Score {
			return deduped[i].Score > deduped[j].Score
		}
		return deduped[i].Rank < deduped[j].Rank
	})
	return deduped
}

func dedupeStrings(values []string) []string {
	deduped := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		deduped = append(deduped, trimmed)
	}
	return deduped
}
