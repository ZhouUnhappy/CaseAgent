package document

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"caseagent/internal/ai"
	"caseagent/internal/config"
	"caseagent/internal/db"
	"caseagent/internal/db/models"
	dbvector "caseagent/internal/db/vector"
	"caseagent/internal/indexing"
	markdowncleaner "caseagent/internal/markdown"

	"github.com/cloudwego/eino/components/embedding"
	"github.com/uptrace/bun"
)

type Service struct {
	db        bun.IDB
	embedding embedding.Embedder
}

func New(ctx context.Context, db bun.IDB) (*Service, error) {
	cfg := config.Get()
	embedder, err := ai.NewEmbedder(ctx, cfg.Model.Embedding)
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

	cleanedContent = markdowncleaner.StripBase64Images(cleanedContent)
	if err := s.updateDocumentContent(ctx, docID, cleanedContent); err != nil {
		return fmt.Errorf("failed to persist cleaned document content: %w", err)
	}
	chunks := s.splitByHeaders(cleanedContent)
	if len(chunks) == 0 {
		return fmt.Errorf("no valid document chunks generated")
	}

	tenantID, _ := db.TenantFromContext(ctx)
	profile := indexing.CurrentProfile()
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
			TenantID:     tenantID,
			DocumentID:   docID,
			ParentDocID:  docID,
			Content:      chunk,
			Embedding:    dbvector.New(embedding32),
			IndexProfile: profile.Name,
			IndexVersion: profile.Version,
			CreatedAt:    time.Now(),
		}

		_, err = s.db.NewInsert().Model(docChunk).Exec(ctx)
		if err != nil {
			return fmt.Errorf("failed to store document chunk: %w", err)
		}
	}

	return nil
}

func (s *Service) ReprocessDocument(ctx context.Context, docID int) (err error) {
	defer func() {
		if err != nil {
			if updateErr := s.updateDocumentStatus(ctx, docID, "failed"); updateErr != nil {
				err = errorsJoin(err, fmt.Errorf("failed to update document status: %w", updateErr))
			}
		}
	}()

	document := &models.Document{}
	if err = s.db.NewSelect().Model(document).Where("id = ?", docID).Scan(ctx); err != nil {
		return fmt.Errorf("failed to load document: %w", err)
	}

	content := document.Content
	if document.Source == "gdrive" {
		if strings.TrimSpace(document.FileID) == "" {
			return fmt.Errorf("document %d is missing Google Drive file ID", docID)
		}
		fetchedContent, fetchErr := s.fetchFromGoogleDrive(ctx, document.FileID)
		if fetchErr != nil {
			return fmt.Errorf("failed to fetch latest Google Drive content: %w", fetchErr)
		}
		content = fetchedContent
		if err = s.updateDocumentContent(ctx, docID, content); err != nil {
			return fmt.Errorf("failed to persist Google Drive content: %w", err)
		}
	}

	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("document %d has no stored content to reprocess", docID)
	}

	if err = s.clearDocumentChunks(ctx, docID); err != nil {
		return err
	}

	return s.ProcessDocument(ctx, docID, content, "")
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
		filtered = append(filtered, splitLargeChunk(chunk, indexing.DocumentMaxChunkRunes)...)
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

func (s *Service) updateDocumentContent(ctx context.Context, docID int, content string) error {
	_, err := s.db.NewUpdate().Model(&models.Document{}).
		Set("content = ?", content).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", docID).
		Exec(ctx)
	return err
}

func (s *Service) clearDocumentChunks(ctx context.Context, docID int) error {
	if _, err := s.db.NewDelete().Model((*models.DocumentChunk)(nil)).Where("document_id = ?", docID).Exec(ctx); err != nil {
		return fmt.Errorf("failed to clear document chunks: %w", err)
	}
	return nil
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

func splitLargeChunk(chunk string, maxRunes int) []string {
	if maxRunes <= 0 || len([]rune(chunk)) <= maxRunes {
		return []string{chunk}
	}

	parts := strings.Split(chunk, "\n\n")
	results := make([]string, 0, len(parts))
	var builder strings.Builder

	flush := func() {
		trimmed := strings.TrimSpace(builder.String())
		if trimmed == "" {
			return
		}
		results = append(results, trimmed)
		builder.Reset()
	}

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if len([]rune(part)) > maxRunes {
			flush()
			results = append(results, hardSplitByRunes(part, maxRunes)...)
			continue
		}

		candidate := part
		if builder.Len() > 0 {
			candidate = builder.String() + "\n\n" + part
		}
		if len([]rune(candidate)) > maxRunes {
			flush()
			builder.WriteString(part)
			continue
		}

		if builder.Len() > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString(part)
	}

	flush()
	return results
}

func hardSplitByRunes(content string, maxRunes int) []string {
	if maxRunes <= 0 || len([]rune(content)) <= maxRunes {
		return []string{strings.TrimSpace(content)}
	}

	runes := []rune(content)
	segments := make([]string, 0, len(runes)/maxRunes+1)
	for start := 0; start < len(runes); start += maxRunes {
		end := start + maxRunes
		if end > len(runes) {
			end = len(runes)
		}
		segment := strings.TrimSpace(string(runes[start:end]))
		if segment == "" {
			continue
		}
		segments = append(segments, segment)
	}
	return segments
}
