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

	arkembedding "github.com/cloudwego/eino-ext/components/embedding/ark"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/uptrace/bun"
)

type Service struct {
	db        *bun.DB
	embedding embedding.Embedder
}

func New(ctx context.Context, db *bun.DB) (*Service, error) {
	cfg := config.Get()
	if cfg.Model.Embedding.Provider != "ark" {
		return nil, fmt.Errorf("only ark embedding provider is supported, got: %s", cfg.Model.Embedding.Provider)
	}

	embedder, err := arkembedding.NewEmbedder(ctx, &arkembedding.EmbeddingConfig{
		APIKey:    cfg.Model.Embedding.APIKey,
		AccessKey: cfg.Model.Embedding.AccessKey,
		SecretKey: cfg.Model.Embedding.SecretKey,
		BaseURL:   cfg.Model.Embedding.BaseURL,
		Region:    cfg.Model.Embedding.Region,
		Model:     cfg.Model.Embedding.Model,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize embedding model: %w", err)
	}

	return &Service{
		db:        db,
		embedding: embedder,
	}, nil
}

// ProcessDocument processes a document: removes base64 images, splits, and stores chunks
func (s *Service) ProcessDocument(ctx context.Context, docID int, content string, gwsFileID string) (err error) {
	status := "completed"
	defer func() {
		if err != nil {
			status = "failed"
		}
		if updateErr := s.updateDocumentStatus(ctx, docID, status); updateErr != nil {
			err = errorsJoin(err, fmt.Errorf("failed to update document status: %w", updateErr))
		}
	}()

	cleanedContent := content
	if gwsFileID != "" {
		markdownContent, err := s.fetchFromGoogleDrive(ctx, gwsFileID)
		if err != nil {
			return fmt.Errorf("failed to fetch from Google Drive: %w", err)
		}
		cleanedContent = markdownContent
	}

	cleanedContent = removeBase64Images(cleanedContent)
	chunks := s.splitByHeaders(cleanedContent)
	if len(chunks) == 0 {
		return fmt.Errorf("no valid document chunks generated")
	}

	for _, chunk := range chunks {
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
			DocumentID:  docID,
			ParentDocID: docID,
			Content:     chunk,
			Embedding:   embedding32,
			CreatedAt:   time.Now(),
		}

		_, err = s.db.NewInsert().Model(docChunk).Exec(ctx)
		if err != nil {
			return fmt.Errorf("failed to store document chunk: %w", err)
		}
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

	filtered := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		chunk = strings.TrimSpace(chunk)
		if chunk == "" {
			continue
		}
		filtered = append(filtered, chunk)
	}

	return filtered
}

// fetchFromGoogleDrive fetches markdown content from Google Drive using gws command
func (s *Service) fetchFromGoogleDrive(ctx context.Context, fileID string) (string, error) {
	cfg := config.Get()
	if !cfg.GWS.Enabled {
		return "", fmt.Errorf("gws integration is disabled")
	}

	command := cfg.GWS.Command
	if command == "" {
		command = "gws"
	}

	cmd := exec.CommandContext(ctx, command, "drive", "files", "export", "--params", fmt.Sprintf(`{"fileId": "%s", "mimeType": "text/markdown"}`, fileID))

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("gws command failed: %w, output: %s", err, string(output))
	}

	return string(output), nil
}

func (s *Service) updateDocumentStatus(ctx context.Context, docID int, status string) error {
	_, err := s.db.NewUpdate().Model(&models.Document{}).
		Set("status = ?", status).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", docID).
		Exec(ctx)
	return err
}

func errorsJoin(base error, next error) error {
	if base == nil {
		return next
	}
	if next == nil {
		return base
	}
	return fmt.Errorf("%v; %w", base, next)
}
