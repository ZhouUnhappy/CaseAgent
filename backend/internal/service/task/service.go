package task

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	agentservice "caseagent/internal/service/agent"
	retrievalservice "caseagent/internal/service/retrieval"

	"caseagent/internal/db/models"

	"github.com/uptrace/bun"
)

type Service struct {
	db *bun.DB
}

type generatedSection struct {
	Section string           `json:"section"`
	Cases   []map[string]any `json:"cases"`
}

func New(db *bun.DB) *Service {
	return &Service{db: db}
}

func (s *Service) AnalyzeTask(ctx context.Context, taskID int) (err error) {
	defer func() {
		if err != nil {
			_ = s.updateTaskStatus(ctx, taskID, models.TaskStatusFailed)
		}
	}()

	task, err := s.getTask(ctx, taskID)
	if err != nil {
		return err
	}

	requirements, err := s.loadRequirements(ctx, task.ProjectID, task.DocumentIDs)
	if err != nil {
		return err
	}

	knowledgeEntries, err := s.listKnowledge(ctx)
	if err != nil {
		return err
	}

	products, modules := inferAffectedKnowledge(requirements, knowledgeEntries)
	if len(products) == 0 && len(modules) == 0 {
		products, modules = inferAffectedKnowledgeWithRetrieval(ctx, s.db, requirements)
	}

	return s.updateTaskAnalysis(ctx, taskID, products, modules, models.TaskStatusAwaitingReview)
}

func (s *Service) GenerateCases(ctx context.Context, taskID int) (err error) {
	if err = s.updateTaskStatus(ctx, taskID, models.TaskStatusGenerating); err != nil {
		return err
	}

	defer func() {
		if err != nil {
			_ = s.updateTaskStatus(ctx, taskID, models.TaskStatusFailed)
		}
	}()

	task, err := s.getTask(ctx, taskID)
	if err != nil {
		return err
	}

	requirements, err := s.loadRequirements(ctx, task.ProjectID, task.DocumentIDs)
	if err != nil {
		return err
	}

	knowledgeEntries, err := s.loadRelevantKnowledge(ctx, task.AffectedProducts, task.AffectedModules)
	if err != nil {
		return err
	}
	retrievedEntries := retrieveKnowledgeFallback(ctx, s.db, requirements, task.AffectedProducts, task.AffectedModules)
	knowledgeEntries = mergeKnowledgeEntries(knowledgeEntries, retrievedEntries)
	requirementsContext := buildRequirementsContext(ctx, s.db, requirements, task.DocumentIDs, task.AffectedProducts, task.AffectedModules)

	agentSvc, err := agentservice.New(ctx, &agentservice.Config{})
	if err != nil {
		return fmt.Errorf("failed to initialize agent service: %w", err)
	}

	rawCases, err := agentSvc.GenerateCases(ctx, requirementsContext, formatKnowledgeContext(knowledgeEntries))
	if err != nil {
		return fmt.Errorf("failed to generate cases: %w", err)
	}

	sections, err := parseGeneratedSections(rawCases)
	if err != nil {
		return fmt.Errorf("failed to parse generated cases: %w", err)
	}
	sections = dedupeGeneratedSections(sections)
	sections = attachCaseContext(sections, task.AffectedProducts, task.AffectedModules)

	if len(sections) == 0 {
		return fmt.Errorf("no test cases generated")
	}

	if err = s.persistGeneratedCases(ctx, taskID, sections); err != nil {
		return err
	}

	return s.updateTaskStatus(ctx, taskID, models.TaskStatusCompleted)
}

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
	if err := s.db.NewSelect().Model(&entries).OrderExpr("type ASC, name ASC").Scan(ctx); err != nil {
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

func (s *Service) persistGeneratedCases(ctx context.Context, taskID int, sections []generatedSection) error {
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewDelete().Model(&models.TestCase{}).Where("task_id = ?", taskID).Exec(ctx); err != nil {
			return fmt.Errorf("failed to clear existing test cases: %w", err)
		}

		now := time.Now()
		for _, section := range sections {
			payload, err := json.Marshal(section.Cases)
			if err != nil {
				return fmt.Errorf("failed to marshal test cases for section %s: %w", section.Section, err)
			}

			testCase := &models.TestCase{
				TaskID:    taskID,
				Section:   section.Section,
				Cases:     string(payload),
				Status:    models.TestCaseStatusDraft,
				CreatedAt: now,
				UpdatedAt: now,
			}

			if _, err := tx.NewInsert().Model(testCase).Exec(ctx); err != nil {
				return fmt.Errorf("failed to store test cases for section %s: %w", section.Section, err)
			}
		}

		return nil
	})
}

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

func inferAffectedKnowledgeWithRetrieval(ctx context.Context, db *bun.DB, requirements string) ([]string, []string) {
	queries := buildKnowledgeQueries(requirements, nil, nil)
	results, err := retrievalservice.New(db).SearchKnowledgeMultiQuery(ctx, queries, 6, "")
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

func retrieveKnowledgeFallback(ctx context.Context, db *bun.DB, requirements string, products []string, modules []string) []models.KnowledgeBase {
	queries := buildKnowledgeQueries(requirements, products, modules)
	results, err := retrievalservice.New(db).SearchKnowledgeMultiQuery(ctx, queries, 6, "")
	if err != nil {
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

func buildRequirementsContext(ctx context.Context, db *bun.DB, requirements string, documentIDs []int, products []string, modules []string) string {
	queries := buildDocumentQueries(requirements, products, modules)
	if len(queries) == 0 || len(documentIDs) == 0 {
		return requirements
	}

	results, err := retrievalservice.New(db).SearchDocumentsMultiQuery(ctx, queries, 4, documentIDs)
	if err != nil || len(results) == 0 {
		return requirements
	}

	var builder strings.Builder
	builder.WriteString("## 需求命中上下文（基于文档检索）\n\n")
	for _, result := range results {
		builder.WriteString("### 文档：")
		builder.WriteString(result.Name)
		builder.WriteString("\n\n")

		chunks := dedupeNonEmptyStrings(result.MatchedChunks)
		maxChunks := 3
		if len(chunks) < maxChunks {
			maxChunks = len(chunks)
		}
		for idx := 0; idx < maxChunks; idx++ {
			builder.WriteString("- ")
			builder.WriteString(chunks[idx])
			builder.WriteString("\n")
		}
		builder.WriteString("\n")
	}

	context := strings.TrimSpace(builder.String())
	if context == "" {
		return requirements
	}

	// If retrieval context is too short, append original requirements as fallback context.
	if len([]rune(context)) < 400 {
		return context + "\n\n## 完整需求文档\n\n" + requirements
	}
	return context
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

func parseGeneratedSections(raw string) ([]generatedSection, error) {
	cleaned := extractJSONPayload(raw)
	if cleaned == "" {
		return nil, fmt.Errorf("empty model response")
	}

	var sections []generatedSection
	if err := json.Unmarshal([]byte(cleaned), &sections); err == nil && len(sections) > 0 {
		if normalized := normalizeSections(sections); len(normalized) > 0 {
			return normalized, nil
		}
	}

	var singleSection generatedSection
	if err := json.Unmarshal([]byte(cleaned), &singleSection); err == nil && (singleSection.Section != "" || len(singleSection.Cases) > 0) {
		if normalized := normalizeSections([]generatedSection{singleSection}); len(normalized) > 0 {
			return normalized, nil
		}
	}

	var flatCases []map[string]any
	if err := json.Unmarshal([]byte(cleaned), &flatCases); err == nil && len(flatCases) > 0 {
		return groupFlatCases(flatCases), nil
	}

	return nil, fmt.Errorf("response is not valid sectioned or flat JSON")
}

func normalizeSections(sections []generatedSection) []generatedSection {
	normalized := make([]generatedSection, 0, len(sections))
	for _, section := range sections {
		name := strings.TrimSpace(section.Section)
		if name == "" {
			name = "未分类"
		}

		cases := make([]map[string]any, 0, len(section.Cases))
		for _, item := range section.Cases {
			if len(item) == 0 {
				continue
			}
			cases = append(cases, normalizeCase(item, name))
		}

		if len(cases) == 0 {
			continue
		}

		normalized = append(normalized, generatedSection{
			Section: name,
			Cases:   cases,
		})
	}

	return normalized
}

func groupFlatCases(flatCases []map[string]any) []generatedSection {
	grouped := make(map[string][]map[string]any)
	order := make([]string, 0, len(flatCases))

	for _, item := range flatCases {
		section := firstNonEmptyString(item["section"], item["type"])
		if section == "" {
			section = "未分类"
		}

		if _, ok := grouped[section]; !ok {
			order = append(order, section)
		}
		grouped[section] = append(grouped[section], normalizeCase(item, section))
	}

	sections := make([]generatedSection, 0, len(grouped))
	for _, section := range order {
		sections = append(sections, generatedSection{
			Section: section,
			Cases:   grouped[section],
		})
	}

	return sections
}

func dedupeGeneratedSections(sections []generatedSection) []generatedSection {
	seen := make(map[string]struct{})
	deduped := make([]generatedSection, 0, len(sections))

	for _, section := range sections {
		filteredCases := make([]map[string]any, 0, len(section.Cases))
		for _, caseItem := range section.Cases {
			signature := caseSignature(caseItem)
			if signature == "" {
				continue
			}
			if _, ok := seen[signature]; ok {
				continue
			}
			seen[signature] = struct{}{}
			filteredCases = append(filteredCases, caseItem)
		}

		if len(filteredCases) == 0 {
			continue
		}

		deduped = append(deduped, generatedSection{
			Section: section.Section,
			Cases:   filteredCases,
		})
	}

	return deduped
}

func caseSignature(item map[string]any) string {
	title := normalizeMatchText(stringValue(item["title"]))
	preconds := normalizeMatchText(stringValue(item["custom_preconds"]))
	steps := normalizeStepSignatures(item["custom_steps_separated"])

	if title == "" && len(steps) == 0 {
		return ""
	}

	return title + "|" + preconds + "|" + strings.Join(steps, "||")
}

func attachCaseContext(sections []generatedSection, products []string, modules []string) []generatedSection {
	if len(sections) == 0 {
		return sections
	}

	normalizedProducts := append([]string{}, products...)
	normalizedModules := append([]string{}, modules...)

	result := make([]generatedSection, 0, len(sections))
	for _, section := range sections {
		cases := make([]map[string]any, 0, len(section.Cases))
		for _, item := range section.Cases {
			if item == nil {
				continue
			}

			cloned := make(map[string]any, len(item)+3)
			for key, value := range item {
				cloned[key] = value
			}

			if _, ok := cloned["affected_products"]; !ok {
				cloned["affected_products"] = normalizedProducts
			}
			if _, ok := cloned["affected_modules"]; !ok {
				cloned["affected_modules"] = normalizedModules
			}
			if _, ok := cloned["section"]; !ok {
				cloned["section"] = section.Section
			}

			cases = append(cases, cloned)
		}

		result = append(result, generatedSection{
			Section: section.Section,
			Cases:   cases,
		})
	}

	return result
}

func normalizeStepSignatures(value any) []string {
	normalize := func(step map[string]any) string {
		content := normalizeMatchText(firstNonEmptyString(step["content"], step["step"]))
		expected := normalizeMatchText(firstNonEmptyString(step["expected"], step["result"]))
		return content + "=>" + expected
	}

	signatures := make([]string, 0)

	switch typed := value.(type) {
	case []map[string]any:
		for _, step := range typed {
			signatures = append(signatures, normalize(step))
		}
	case []any:
		for _, raw := range typed {
			stepMap, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			signatures = append(signatures, normalize(stepMap))
		}
	}

	return signatures
}

func normalizeCase(item map[string]any, fallbackSection string) map[string]any {
	title := stringValue(item["title"])
	if title == "" {
		title = fmt.Sprintf("[%s] 自动生成用例", fallbackSection)
	}

	customPreconds := stringValue(item["custom_preconds"])
	if customPreconds == "" {
		customPreconds = stringValue(item["description"])
	}

	normalized := map[string]any{
		"title":                  title,
		"priority_id":            intValue(item["priority_id"], 3),
		"custom_preconds":        customPreconds,
		"custom_steps_separated": normalizeSteps(item),
	}

	return normalized
}

func normalizeSteps(item map[string]any) []map[string]any {
	if value, ok := item["custom_steps_separated"]; ok {
		if steps := normalizeStepList(value); len(steps) > 0 {
			return steps
		}
	}

	expectedResult := stringValue(item["expected_result"])
	if value, ok := item["steps"]; ok {
		if steps := normalizeSimpleSteps(value, expectedResult); len(steps) > 0 {
			return steps
		}
	}

	description := stringValue(item["description"])
	if description != "" || expectedResult != "" {
		return []map[string]any{{
			"content":  firstNonEmptyString(description, "执行测试步骤"),
			"expected": firstNonEmptyString(expectedResult, "结果符合预期"),
		}}
	}

	return []map[string]any{{
		"content":  "执行测试步骤",
		"expected": "结果符合预期",
	}}
}

func normalizeStepList(value any) []map[string]any {
	rawSteps, ok := value.([]any)
	if !ok {
		return nil
	}

	steps := make([]map[string]any, 0, len(rawSteps))
	for _, raw := range rawSteps {
		stepMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		content := firstNonEmptyString(stepMap["content"], stepMap["step"])
		expected := firstNonEmptyString(stepMap["expected"], stepMap["result"])
		steps = append(steps, map[string]any{
			"content":  firstNonEmptyString(content, "执行测试步骤"),
			"expected": expected,
		})
	}

	return steps
}

func normalizeSimpleSteps(value any, expectedResult string) []map[string]any {
	rawSteps, ok := value.([]any)
	if !ok {
		return nil
	}

	steps := make([]map[string]any, 0, len(rawSteps))
	for idx, raw := range rawSteps {
		switch typed := raw.(type) {
		case string:
			step := map[string]any{
				"content":  typed,
				"expected": "",
			}
			if idx == len(rawSteps)-1 && expectedResult != "" {
				step["expected"] = expectedResult
			}
			steps = append(steps, step)
		case map[string]any:
			step := map[string]any{
				"content":  firstNonEmptyString(typed["content"], typed["step"], "执行测试步骤"),
				"expected": firstNonEmptyString(typed["expected"], typed["result"]),
			}
			if idx == len(rawSteps)-1 && step["expected"] == "" && expectedResult != "" {
				step["expected"] = expectedResult
			}
			steps = append(steps, step)
		}
	}

	return steps
}

func extractJSONPayload(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "```") {
		lines := strings.Split(trimmed, "\n")
		if len(lines) > 0 && strings.HasPrefix(strings.TrimSpace(lines[0]), "```") {
			lines = lines[1:]
		}
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "```" {
			lines = lines[:len(lines)-1]
		}
		trimmed = strings.Join(lines, "\n")
	}

	trimmed = strings.TrimSpace(trimmed)

	startArray := strings.Index(trimmed, "[")
	startObject := strings.Index(trimmed, "{")
	start := firstPositiveIndex(startArray, startObject)
	if start == -1 {
		return trimmed
	}

	endArray := strings.LastIndex(trimmed, "]")
	endObject := strings.LastIndex(trimmed, "}")
	end := max(endArray, endObject)
	if end <= start {
		return trimmed[start:]
	}

	return strings.TrimSpace(trimmed[start : end+1])
}

func normalizeMatchText(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), ""))
}

func mapKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func stringValue(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func firstNonEmptyString(values ...any) string {
	for _, value := range values {
		if text := stringValue(value); text != "" {
			return text
		}
	}
	return ""
}

func intValue(value any, fallback int) int {
	switch typed := value.(type) {
	case int:
		if typed > 0 {
			return typed
		}
	case int32:
		if typed > 0 {
			return int(typed)
		}
	case int64:
		if typed > 0 {
			return int(typed)
		}
	case float32:
		if typed > 0 {
			return int(typed)
		}
	case float64:
		if typed > 0 {
			return int(typed)
		}
	}
	return fallback
}

func firstPositiveIndex(values ...int) int {
	best := -1
	for _, value := range values {
		if value < 0 {
			continue
		}
		if best == -1 || value < best {
			best = value
		}
	}
	return best
}

func max(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
