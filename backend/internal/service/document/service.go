package document

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"caseagent/internal/config"
	"caseagent/internal/db/models"

	"github.com/cloudwego/eino-ext/components/embedding/openai"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/uptrace/bun"
)

type Service struct {
	db        *bun.DB
	embedding embedding.Embedder
}

func New(ctx context.Context, db *bun.DB) (*Service, error) {
	cfg := config.Get()

	// Initialize embedding model based on provider
	var embedder embedding.Embedder
	var err error

	switch cfg.Model.Embedding.Provider {
	case "openai":
		embedder, err = openai.NewEmbedder(ctx, &openai.EmbeddingConfig{
			APIKey:  cfg.Model.Embedding.APIKey,
			BaseURL: cfg.Model.Embedding.BaseURL,
			Model:   cfg.Model.Embedding.Model,
		})
	default:
		return nil, fmt.Errorf("unsupported embedding provider: %s", cfg.Model.Embedding.Provider)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to initialize embedding model: %w", err)
	}

	return &Service{
		db:        db,
		embedding: embedder,
	}, nil
}

// ProcessDocument processes a document: removes base64 images, splits, and stores chunks
func (s *Service) ProcessDocument(ctx context.Context, docID int, content string, gwsFileID string) error {
	// Step 1: Remove base64 images
	cleanedContent := removeBase64Images(content)

	// Step 2: If Google Drive ID is provided, fetch the content
	if gwsFileID != "" {
		markdownContent, err := s.fetchFromGoogleDrive(ctx, gwsFileID)
		if err != nil {
			return fmt.Errorf("failed to fetch from Google Drive: %w", err)
		}
		cleanedContent = markdownContent
	}

	// Step 3: Split document by headers
	chunks := s.splitByHeaders(cleanedContent)

	// Step 4: Generate embeddings for each chunk and store
	for _, chunk := range chunks {
		// Generate embedding
		embResult, err := s.embedding.EmbedStrings(ctx, []string{chunk})
		if err != nil {
			return fmt.Errorf("failed to generate embedding: %w", err)
		}

		if len(embResult) == 0 || len(embResult[0]) == 0 {
			return fmt.Errorf("empty embedding result")
		}

		// Convert []float64 to []float32
		embedding32 := make([]float32, len(embResult[0]))
		for i, v := range embResult[0] {
			embedding32[i] = float32(v)
		}

		docChunk := &models.DocumentChunk{
			DocumentID: docID,
			Content:    chunk,
			Embedding:  embedding32,
			CreatedAt:  time.Now(),
		}

		_, err = s.db.NewInsert().Model(docChunk).Exec(ctx)
		if err != nil {
			return fmt.Errorf("failed to store document chunk: %w", err)
		}
	}

	// Update document status
	_, err := s.db.NewUpdate().Model(&models.Document{}).
		Set("status = ?", "completed").
		Set("updated_at = ?", time.Now()).
		Where("id = ?", docID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update document status: %w", err)
	}

	return nil
}

// removeBase64Images removes base64 encoded images from markdown content
func removeBase64Images(content string) string {
	// Pattern to match base64 images: ![alt](data:image/...;base64,...)
	pattern := regexp.MustCompile(`!\[.*?\]\(data:image/[^;]+;base64,[^)]+\)`)
	return pattern.ReplaceAllString(content, "")
}

// splitByHeaders splits markdown content by ## and ### headers
func (s *Service) splitByHeaders(content string) []string {
	lines := strings.Split(content, "\n")
	var chunks []string
	var currentChunk []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check if line is a header (## or ###)
		if strings.HasPrefix(trimmed, "##") && !strings.HasPrefix(trimmed, "####") {
			// Save previous chunk if not empty
			if len(currentChunk) > 0 {
				chunks = append(chunks, strings.Join(currentChunk, "\n"))
			}
			// Start new chunk with the header
			currentChunk = []string{line}
		} else {
			currentChunk = append(currentChunk, line)
		}
	}

	// Add the last chunk
	if len(currentChunk) > 0 {
		chunks = append(chunks, strings.Join(currentChunk, "\n"))
	}

	return chunks
}

// fetchFromGoogleDrive fetches markdown content from Google Drive using gws command
func (s *Service) fetchFromGoogleDrive(ctx context.Context, fileID string) (string, error) {
	// Execute gws command to export file
	cmd := exec.CommandContext(ctx, "gws", "drive", "files", "export", "--params", fmt.Sprintf(`{"fileId": "%s", "mimeType": "text/markdown"}`, fileID))

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("gws command failed: %w, output: %s", err, string(output))
	}

	return string(output), nil
}
