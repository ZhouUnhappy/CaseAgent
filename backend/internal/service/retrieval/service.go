package retrieval

import (
	"context"
	"fmt"
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
