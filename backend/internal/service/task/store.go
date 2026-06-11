package task

import (
	"context"
	"fmt"
	"strings"
	"time"

	"caseagent/internal/db"
	"caseagent/internal/db/models"

	"github.com/uptrace/bun"
)

func (s *Service) getTask(ctx context.Context, taskID int) (*models.CaseGenerationTask, error) {
	task := &models.CaseGenerationTask{}
	if err := s.db.NewSelect().Model(task).Where("id = ?", taskID).Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to load task: %w", err)
	}
	return task, nil
}

func (s *Service) updateTaskStatus(ctx context.Context, taskID int, status string) error {
	_, err := s.db.NewUpdate().Model(&models.CaseGenerationTask{}).
		Set("status = ?", status).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", taskID).
		Exec(ctx)
	return err
}

func (s *Service) updateTaskAnalysis(ctx context.Context, taskID int, products []string, modules []string, status string) error {
	_, err := s.db.NewUpdate().Model(&models.CaseGenerationTask{}).
		Set("affected_products = ?", products).
		Set("affected_modules = ?", modules).
		Set("status = ?", status).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", taskID).
		Exec(ctx)
	return err
}

func (s *Service) loadRequirements(ctx context.Context, projectID int, documentIDs []int) (string, error) {
	if len(documentIDs) == 0 {
		return "", fmt.Errorf("no documents selected")
	}

	var documents []models.Document
	if err := s.db.NewSelect().
		Model(&documents).
		Where("project_id = ?", projectID).
		Where("id IN (?)", bun.In(documentIDs)).
		Scan(ctx); err != nil {
		return "", fmt.Errorf("failed to load documents: %w", err)
	}

	docByID := make(map[int]models.Document, len(documents))
	for _, doc := range documents {
		docByID[doc.ID] = doc
	}

	var chunks []models.DocumentChunk
	if err := s.db.NewSelect().
		Model(&chunks).
		Where("document_id IN (?)", bun.In(documentIDs)).
		OrderExpr("document_id ASC, id ASC").
		Scan(ctx); err != nil {
		return "", fmt.Errorf("failed to load document chunks: %w", err)
	}

	chunksByDoc := make(map[int][]string, len(documentIDs))
	for _, chunk := range chunks {
		chunksByDoc[chunk.DocumentID] = append(chunksByDoc[chunk.DocumentID], strings.TrimSpace(chunk.Content))
	}

	var builder strings.Builder
	for _, documentID := range documentIDs {
		doc, ok := docByID[documentID]
		if !ok {
			return "", fmt.Errorf("document %d does not belong to project %d", documentID, projectID)
		}

		builder.WriteString("## 文档：")
		builder.WriteString(doc.Name)
		builder.WriteString("\n\n")

		docChunks := chunksByDoc[documentID]
		if len(docChunks) == 0 {
			return "", fmt.Errorf("document %d has no processed chunks", documentID)
		}

		for _, content := range docChunks {
			if content == "" {
				continue
			}
			builder.WriteString(content)
			builder.WriteString("\n\n")
		}
	}

	return strings.TrimSpace(builder.String()), nil
}

func (s *Service) listKnowledge(ctx context.Context) ([]models.KnowledgeBase, error) {
	var entries []models.KnowledgeBase
	if err := s.db.NewSelect().
		Model(&entries).
		Where("status = ?", models.KnowledgeStatusCompleted).
		Where("(expires_at IS NULL OR expires_at > ?)", time.Now()).
		Where("duplicate_of_id IS NULL").
		OrderExpr("type ASC, name ASC").
		Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to load knowledge base: %w", err)
	}
	return entries, nil
}

func (s *Service) loadRelevantKnowledge(ctx context.Context, products []string, modules []string) ([]models.KnowledgeBase, error) {
	entries, err := s.listKnowledge(ctx)
	if err != nil {
		return nil, err
	}

	if len(products) == 0 && len(modules) == 0 {
		return nil, nil
	}

	productSet := make(map[string]struct{}, len(products))
	moduleSet := make(map[string]struct{}, len(modules))
	for _, name := range products {
		productSet[name] = struct{}{}
	}
	for _, name := range modules {
		moduleSet[name] = struct{}{}
	}

	filtered := make([]models.KnowledgeBase, 0, len(entries))
	for _, entry := range entries {
		switch entry.Type {
		case "product":
			if _, ok := productSet[entry.Name]; ok {
				filtered = append(filtered, entry)
			}
		case "module":
			if _, ok := moduleSet[entry.Name]; ok {
				filtered = append(filtered, entry)
			}
		}
	}

	return filtered, nil
}

func (s *Service) persistGeneratedCases(ctx context.Context, taskID int, sections []generatedSection, sourceContext map[string]any) error {
	if _, err := s.db.NewDelete().Model(&models.TestCase{}).Where("task_id = ?", taskID).Exec(ctx); err != nil {
		return fmt.Errorf("failed to clear existing test cases: %w", err)
	}

	tenantID, _ := db.TenantFromContext(ctx)
	now := time.Now()
	for _, section := range sections {
		testCase := &models.TestCase{
			TenantID:      tenantID,
			TaskID:        taskID,
			Section:       section.Section,
			Cases:         section.Cases,
			SourceContext: sourceContext,
			Status:        models.TestCaseStatusDraft,
			CreatedAt:     now,
			UpdatedAt:     now,
		}

		if _, err := s.db.NewInsert().Model(testCase).Exec(ctx); err != nil {
			return fmt.Errorf("failed to store test cases for section %s: %w", section.Section, err)
		}
	}

	return nil
}
