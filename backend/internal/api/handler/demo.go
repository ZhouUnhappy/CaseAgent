package handler

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"caseagent/internal/db/models"
	documentservice "caseagent/internal/service/document"
	jobservice "caseagent/internal/service/job"
	knowledgeservice "caseagent/internal/service/knowledge"
	taskservice "caseagent/internal/service/task"

	"github.com/gin-gonic/gin"
)

const (
	demoProjectPrefix        = "Demo CaseAgent"
	demoKnowledgeAlias       = "CaseAgent demo fixture"
	demoKnowledgeSource      = "demo"
	demoProductName          = "CaseAgent Cloud"
	demoModuleName           = "控制平面"
	demoRequirementFixture   = "requirement.md"
	demoProductFixture       = "product_knowledge.md"
	demoModuleFixture        = "module_knowledge.md"
	demoBootstrapDescription = "Created by demo_console"
)

type DemoBootstrapRequest struct {
	RunToken    string `json:"run_token"`
	FrontendURL string `json:"frontend_url"`
}

type DemoResetResult struct {
	MatchedProjects  int `json:"matched_projects"`
	DeletedProjects  int `json:"deleted_projects"`
	MatchedKnowledge int `json:"matched_knowledge"`
	DeletedKnowledge int `json:"deleted_knowledge"`
}

type DemoBootstrapResult struct {
	TenantID           int              `json:"tenant_id"`
	TenantSlug         string           `json:"tenant_slug"`
	RunToken           string           `json:"run_token"`
	ProjectID          int              `json:"project_id"`
	DocumentID         int              `json:"document_id"`
	ProductKnowledgeID int              `json:"product_knowledge_id"`
	ModuleKnowledgeID  int              `json:"module_knowledge_id"`
	KnowledgeIDs       []int            `json:"knowledge_ids"`
	TaskID             int              `json:"task_id"`
	ProjectPath        string           `json:"project_path"`
	TaskPath           string           `json:"task_path"`
	ProjectURL         string           `json:"project_url"`
	TaskURL            string           `json:"task_url"`
	Reset              *DemoResetResult `json:"reset,omitempty"`
}

type demoFixtures struct {
	Requirement      string
	ProductKnowledge string
	ModuleKnowledge  string
}

func (h *Handler) ResetDemo(c *gin.Context) {
	result, err := resetDemoData(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) BootstrapDemo(c *gin.Context) {
	var req DemoBootstrapRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := bootstrapDemoData(c, req, nil)
	if err != nil {
		writeDemoBootstrapError(c, err)
		return
	}
	c.JSON(http.StatusCreated, result)
}

func (h *Handler) FreshDemo(c *gin.Context) {
	var req DemoBootstrapRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	reset, err := resetDemoData(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	result, err := bootstrapDemoData(c, req, &reset)
	if err != nil {
		writeDemoBootstrapError(c, err)
		return
	}
	c.JSON(http.StatusCreated, result)
}

func resetDemoData(c *gin.Context) (DemoResetResult, error) {
	var projects []models.Project
	if err := DBFromContext(c).NewSelect().
		Model(&projects).
		Where("name LIKE ? OR description LIKE ? OR description LIKE ?",
			demoProjectPrefix+" %",
			"%scripts/demo_bootstrap.sh%",
			"%demo_console%",
		).
		Scan(c); err != nil {
		return DemoResetResult{}, fmt.Errorf("list demo projects: %w", err)
	}

	var knowledge []models.KnowledgeBase
	if err := DBFromContext(c).NewSelect().
		Model(&knowledge).
		Where("source = ? OR metadata::text LIKE ?", demoKnowledgeSource, "%"+demoKnowledgeAlias+"%").
		Scan(c); err != nil {
		return DemoResetResult{}, fmt.Errorf("list demo knowledge: %w", err)
	}

	result := DemoResetResult{
		MatchedProjects:  len(projects),
		MatchedKnowledge: len(knowledge),
	}
	for _, project := range projects {
		if _, err := DBFromContext(c).NewDelete().Model(&models.Project{}).Where("id = ?", project.ID).Exec(c); err != nil {
			return result, fmt.Errorf("delete demo project %d: %w", project.ID, err)
		}
		result.DeletedProjects++
	}
	for _, entry := range knowledge {
		if _, err := DBFromContext(c).NewDelete().Model(&models.KnowledgeBase{}).Where("id = ?", entry.ID).Exec(c); err != nil {
			return result, fmt.Errorf("delete demo knowledge %d: %w", entry.ID, err)
		}
		result.DeletedKnowledge++
	}
	return result, nil
}

func bootstrapDemoData(c *gin.Context, req DemoBootstrapRequest, reset *DemoResetResult) (DemoBootstrapResult, error) {
	tenantID, _ := TenantIDFromContext(c)
	runToken := demoRunToken(req.RunToken, time.Now())
	fixtures, err := loadDemoFixtures()
	if err != nil {
		return DemoBootstrapResult{}, err
	}

	now := time.Now()
	project := &models.Project{
		TenantID:    tenantID,
		Name:        demoProjectPrefix + " " + runToken,
		Description: demoBootstrapDescription + " (run_token=" + runToken + ")",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if _, err := DBFromContext(c).NewInsert().Model(project).Exec(c); err != nil {
		return DemoBootstrapResult{}, fmt.Errorf("create demo project: %w", err)
	}

	document, err := createProcessedDemoDocument(c, tenantID, project.ID, runToken, fixtures.Requirement)
	if err != nil {
		return DemoBootstrapResult{}, err
	}
	productKnowledge, err := createProcessedDemoKnowledge(c, tenantID, "product", demoProductName, runToken, fixtures.ProductKnowledge)
	if err != nil {
		return DemoBootstrapResult{}, err
	}
	moduleKnowledge, err := createProcessedDemoKnowledge(c, tenantID, "module", demoModuleName, runToken, fixtures.ModuleKnowledge)
	if err != nil {
		return DemoBootstrapResult{}, err
	}

	task, err := taskservice.New(DBFromContext(c)).CreateTask(c, project.ID, []int{document.ID})
	if err != nil {
		return DemoBootstrapResult{}, err
	}
	if _, err := jobservice.New(DBFromContext(c)).Enqueue(c.Request.Context(), jobservice.EnqueueInput{
		TaskID:     task.ID,
		JobType:    models.JobTypeAnalyze,
		MaxRetries: configuredJobMaxRetriesFor(models.JobTypeAnalyze),
	}); err != nil {
		return DemoBootstrapResult{}, fmt.Errorf("enqueue demo analyze job: %w", err)
	}

	projectPath := fmt.Sprintf("/projects/%d", project.ID)
	taskPath := fmt.Sprintf("/tasks/%d", task.ID)
	result := DemoBootstrapResult{
		TenantID:           tenantID,
		TenantSlug:         strings.TrimSpace(c.GetHeader("X-Tenant-ID")),
		RunToken:           runToken,
		ProjectID:          project.ID,
		DocumentID:         document.ID,
		ProductKnowledgeID: productKnowledge.ID,
		ModuleKnowledgeID:  moduleKnowledge.ID,
		KnowledgeIDs:       []int{productKnowledge.ID, moduleKnowledge.ID},
		TaskID:             task.ID,
		ProjectPath:        projectPath,
		TaskPath:           taskPath,
		ProjectURL:         demoFrontendURL(req.FrontendURL, projectPath),
		TaskURL:            demoFrontendURL(req.FrontendURL, taskPath),
		Reset:              reset,
	}
	return result, nil
}

func createProcessedDemoDocument(c *gin.Context, tenantID int, projectID int, runToken string, content string) (*models.Document, error) {
	now := time.Now()
	document := &models.Document{
		TenantID:  tenantID,
		ProjectID: projectID,
		Name:      "Demo Requirement " + runToken,
		Type:      "markdown",
		Source:    "upload",
		Content:   content,
		Status:    models.DocumentStatusProcessing,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if _, err := DBFromContext(c).NewInsert().Model(document).Exec(c); err != nil {
		return nil, fmt.Errorf("create demo document: %w", err)
	}

	svc, err := documentservice.New(c, DBFromContext(c))
	if err != nil {
		return nil, fmt.Errorf("initialize demo document processor: %w", err)
	}
	if err := svc.ProcessDocument(c, document.ID, content, ""); err != nil {
		return nil, fmt.Errorf("process demo document: %w", err)
	}
	document.Status = models.DocumentStatusCompleted
	return document, nil
}

func createProcessedDemoKnowledge(c *gin.Context, tenantID int, kbType string, name string, runToken string, content string) (*models.KnowledgeBase, error) {
	now := time.Now()
	entry := &models.KnowledgeBase{
		TenantID: tenantID,
		Type:     kbType,
		Name:     name,
		Content:  content,
		Metadata: map[string]any{
			"aliases":   []string{demoKnowledgeAlias},
			"run_token": runToken,
			"demo":      "caseagent",
		},
		Source:    demoKnowledgeSource,
		Status:    models.KnowledgeStatusProcessing,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if _, err := DBFromContext(c).NewInsert().Model(entry).Exec(c); err != nil {
		return nil, fmt.Errorf("create demo knowledge: %w", err)
	}

	svc, err := knowledgeservice.New(c, DBFromContext(c))
	if err != nil {
		return nil, fmt.Errorf("initialize demo knowledge processor: %w", err)
	}
	if err := svc.ProcessKnowledge(c, entry.ID, content); err != nil {
		return nil, fmt.Errorf("process demo knowledge: %w", err)
	}
	entry.Status = models.KnowledgeStatusCompleted
	return entry, nil
}

func loadDemoFixtures() (demoFixtures, error) {
	requirement, err := readPublicDemoFixture(demoRequirementFixture)
	if err != nil {
		return demoFixtures{}, err
	}
	productKnowledge, err := readPublicDemoFixture(demoProductFixture)
	if err != nil {
		return demoFixtures{}, err
	}
	moduleKnowledge, err := readPublicDemoFixture(demoModuleFixture)
	if err != nil {
		return demoFixtures{}, err
	}
	return demoFixtures{
		Requirement:      requirement,
		ProductKnowledge: productKnowledge,
		ModuleKnowledge:  moduleKnowledge,
	}, nil
}

func readPublicDemoFixture(name string) (string, error) {
	path, err := publicDemoFixturePath(name)
	if err != nil {
		return "", err
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read public demo fixture %s: %w", name, err)
	}
	return string(bytes), nil
}

func publicDemoFixturePath(name string) (string, error) {
	if strings.Contains(name, "..") || strings.ContainsAny(name, `/\`) {
		return "", fmt.Errorf("invalid public demo fixture name %q", name)
	}

	_, sourceFile, _, ok := runtime.Caller(0)
	candidates := []string{
		filepath.Join("testdata", "i1", name),
		filepath.Join("..", "testdata", "i1", name),
	}
	if ok {
		candidates = append(candidates, filepath.Join(filepath.Dir(sourceFile), "..", "..", "..", "..", "testdata", "i1", name))
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("public demo fixture %s not found", name)
}

func demoRunToken(raw string, now time.Time) string {
	token := strings.TrimSpace(raw)
	if token != "" {
		return token
	}
	return "demo-" + now.Format("20060102150405")
}

func demoFrontendURL(base string, path string) string {
	path = "/" + strings.TrimLeft(path, "/")
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return path
	}
	return base + path
}

func writeDemoBootstrapError(c *gin.Context, err error) {
	var badRequest *taskservice.BadRequestError
	if errors.As(err, &badRequest) {
		c.JSON(http.StatusBadRequest, gin.H{"error": badRequest.Error()})
		return
	}
	var notFound *taskservice.NotFoundError
	if errors.As(err, &notFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": notFound.Error()})
		return
	}
	var conflict *taskservice.ConflictError
	if errors.As(err, &conflict) {
		c.JSON(http.StatusConflict, gin.H{"error": conflict.Error()})
		return
	}
	if strings.Contains(err.Error(), "public demo fixture") || strings.Contains(err.Error(), "invalid public demo fixture") || strings.Contains(err.Error(), "no documents selected") {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}
