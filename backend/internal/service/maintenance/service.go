package maintenance

import (
	"context"
	"fmt"
	"strings"

	"caseagent/internal/config"
	"caseagent/internal/db/models"

	"github.com/uptrace/bun"
)

type Service struct {
	db *bun.DB
}

type VectorHealthReport struct {
	Dimensions int                   `json:"dimensions"`
	Documents  DocumentVectorHealth  `json:"documents"`
	Knowledge  KnowledgeVectorHealth `json:"knowledge"`
}

type DocumentVectorHealth struct {
	Total               int   `json:"total"`
	ProcessingIDs       []int `json:"processing_ids"`
	ReprocessableIDs    []int `json:"reprocessable_ids"`
	BlockedIDs          []int `json:"blocked_ids"`
	NoChunksIDs         []int `json:"no_chunks_ids"`
	MissingEmbeddingIDs []int `json:"missing_embedding_ids"`
	MismatchedVectorIDs []int `json:"mismatched_vector_ids"`
}

type KnowledgeVectorHealth struct {
	Total               int   `json:"total"`
	ProcessingIDs       []int `json:"processing_ids"`
	ReprocessableIDs    []int `json:"reprocessable_ids"`
	BlockedIDs          []int `json:"blocked_ids"`
	MissingEmbeddingIDs []int `json:"missing_embedding_ids"`
	MismatchedVectorIDs []int `json:"mismatched_vector_ids"`
}

type RepairPlan struct {
	DocumentIDs         []int `json:"document_ids"`
	KnowledgeIDs        []int `json:"knowledge_ids"`
	BlockedDocumentIDs  []int `json:"blocked_document_ids"`
	BlockedKnowledgeIDs []int `json:"blocked_knowledge_ids"`
}

type documentVectorRow struct {
	ID                       int    `bun:"id"`
	Source                   string `bun:"source"`
	Content                  string `bun:"content"`
	FileID                   string `bun:"file_id"`
	Status                   string `bun:"status"`
	ChunkCount               int    `bun:"chunk_count"`
	MissingEmbeddingCount    int    `bun:"missing_embedding_count"`
	MismatchedEmbeddingCount int    `bun:"mismatched_embedding_count"`
}

type knowledgeVectorRow struct {
	ID                       int    `bun:"id"`
	Content                  string `bun:"content"`
	Status                   string `bun:"status"`
	MissingEmbeddingCount    int    `bun:"missing_embedding_count"`
	MismatchedEmbeddingCount int    `bun:"mismatched_embedding_count"`
}

func New(db *bun.DB) *Service {
	return &Service{db: db}
}

func (s *Service) VectorHealth(ctx context.Context) (*VectorHealthReport, error) {
	dimensions := config.Get().Model.Embedding.Dimensions
	documents, err := s.documentVectorHealth(ctx, dimensions)
	if err != nil {
		return nil, err
	}
	knowledge, err := s.knowledgeVectorHealth(ctx, dimensions)
	if err != nil {
		return nil, err
	}

	return &VectorHealthReport{
		Dimensions: dimensions,
		Documents:  documents,
		Knowledge:  knowledge,
	}, nil
}

func (s *Service) RepairPlan(ctx context.Context) (*RepairPlan, error) {
	report, err := s.VectorHealth(ctx)
	if err != nil {
		return nil, err
	}

	return &RepairPlan{
		DocumentIDs:         report.Documents.ReprocessableIDs,
		KnowledgeIDs:        report.Knowledge.ReprocessableIDs,
		BlockedDocumentIDs:  report.Documents.BlockedIDs,
		BlockedKnowledgeIDs: report.Knowledge.BlockedIDs,
	}, nil
}

func (s *Service) documentVectorHealth(ctx context.Context, dimensions int) (DocumentVectorHealth, error) {
	var rows []documentVectorRow
	err := s.db.NewRaw(`
		SELECT
			d.id,
			d.source,
			d.content,
			d.file_id,
			d.status,
			COUNT(dc.id) AS chunk_count,
			COALESCE(SUM(CASE WHEN dc.id IS NOT NULL AND dc.embedding IS NULL THEN 1 ELSE 0 END), 0) AS missing_embedding_count,
			COALESCE(SUM(CASE WHEN dc.id IS NOT NULL AND dc.embedding IS NOT NULL AND vector_dims(dc.embedding) <> ? THEN 1 ELSE 0 END), 0) AS mismatched_embedding_count
		FROM documents AS d
		LEFT JOIN document_chunks AS dc ON dc.document_id = d.id
		GROUP BY d.id
		ORDER BY d.id ASC
	`, dimensions).Scan(ctx, &rows)
	if err != nil {
		return DocumentVectorHealth{}, fmt.Errorf("inspect document vector health: %w", err)
	}

	health := DocumentVectorHealth{Total: len(rows)}
	for _, row := range rows {
		needsReprocess, blocked := classifyDocumentVectorRow(row)
		if row.Status == models.DocumentStatusProcessing {
			health.ProcessingIDs = append(health.ProcessingIDs, row.ID)
		}
		if row.ChunkCount == 0 {
			health.NoChunksIDs = append(health.NoChunksIDs, row.ID)
		}
		if row.MissingEmbeddingCount > 0 {
			health.MissingEmbeddingIDs = append(health.MissingEmbeddingIDs, row.ID)
		}
		if row.MismatchedEmbeddingCount > 0 {
			health.MismatchedVectorIDs = append(health.MismatchedVectorIDs, row.ID)
		}
		if blocked {
			health.BlockedIDs = append(health.BlockedIDs, row.ID)
			continue
		}
		if needsReprocess && row.Status != models.DocumentStatusProcessing {
			health.ReprocessableIDs = append(health.ReprocessableIDs, row.ID)
		}
	}

	return health, nil
}

func (s *Service) knowledgeVectorHealth(ctx context.Context, dimensions int) (KnowledgeVectorHealth, error) {
	var rows []knowledgeVectorRow
	err := s.db.NewRaw(`
		SELECT
			id,
			content,
			status,
			CASE WHEN embedding IS NULL THEN 1 ELSE 0 END AS missing_embedding_count,
			CASE WHEN embedding IS NOT NULL AND vector_dims(embedding) <> ? THEN 1 ELSE 0 END AS mismatched_embedding_count
		FROM knowledge_base
		ORDER BY id ASC
	`, dimensions).Scan(ctx, &rows)
	if err != nil {
		return KnowledgeVectorHealth{}, fmt.Errorf("inspect knowledge vector health: %w", err)
	}

	health := KnowledgeVectorHealth{Total: len(rows)}
	for _, row := range rows {
		needsReprocess, blocked := classifyKnowledgeVectorRow(row)
		if row.Status == models.KnowledgeStatusProcessing {
			health.ProcessingIDs = append(health.ProcessingIDs, row.ID)
		}
		if row.MissingEmbeddingCount > 0 {
			health.MissingEmbeddingIDs = append(health.MissingEmbeddingIDs, row.ID)
		}
		if row.MismatchedEmbeddingCount > 0 {
			health.MismatchedVectorIDs = append(health.MismatchedVectorIDs, row.ID)
		}
		if blocked {
			health.BlockedIDs = append(health.BlockedIDs, row.ID)
			continue
		}
		if needsReprocess && row.Status != models.KnowledgeStatusProcessing {
			health.ReprocessableIDs = append(health.ReprocessableIDs, row.ID)
		}
	}

	return health, nil
}

func classifyDocumentVectorRow(row documentVectorRow) (needsReprocess bool, blocked bool) {
	needsReprocess = row.ChunkCount == 0 || row.MissingEmbeddingCount > 0 || row.MismatchedEmbeddingCount > 0
	if !needsReprocess {
		return false, false
	}

	switch row.Source {
	case "upload":
		if strings.TrimSpace(row.Content) == "" {
			return true, true
		}
	case "gdrive":
		if strings.TrimSpace(row.FileID) == "" {
			return true, true
		}
	}

	return true, false
}

func classifyKnowledgeVectorRow(row knowledgeVectorRow) (needsReprocess bool, blocked bool) {
	needsReprocess = row.MissingEmbeddingCount > 0 || row.MismatchedEmbeddingCount > 0
	if !needsReprocess {
		return false, false
	}
	if strings.TrimSpace(row.Content) == "" {
		return true, true
	}
	return true, false
}
