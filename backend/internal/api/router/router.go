package router

import (
	"caseagent/internal/api/handler"
	"caseagent/internal/api/middleware"

	"github.com/gin-gonic/gin"
)

func SetupRouter(h *handler.Handler) *gin.Engine {
	r := gin.Default()

	v1 := r.Group("/api/v1")

	// Tenants: bypass Tenant middleware (creating a tenant doesn't need a
	// tenant context). Still wrap in Tx so DBFromContext works.
	tenants := v1.Group("/tenants")
	tenants.Use(middleware.Tx(h.DB))
	{
		tenants.POST("", h.CreateTenant)
		tenants.GET("", h.ListTenants)
		tenants.PUT("/:slug", h.UpdateTenant)
		tenants.POST("/:slug/archive", h.ArchiveTenant)
		tenants.POST("/:slug/unarchive", h.UnarchiveTenant)
	}

	// All other routes require X-Tenant-ID and run inside a tenant-scoped tx.
	biz := v1.Group("")
	biz.Use(middleware.Tenant(h.DB), middleware.Tx(h.DB))
	{
		projects := biz.Group("/projects")
		{
			projects.POST("", h.CreateProject)
			projects.GET("", h.ListProjects)
			projects.GET("/:id", h.GetProject)
			projects.PUT("/:id", h.UpdateProject)
			projects.DELETE("/:id", h.DeleteProject)

			projects.GET("/:id/documents", h.ListDocuments)
			projects.POST("/:id/documents", h.UploadDocument)

			projects.GET("/:id/tasks", h.ListTasks)
			projects.POST("/:id/tasks", h.CreateGenerationTask)
		}

		biz.GET("/documents/:id", h.GetDocument)
		biz.POST("/documents/:id/reprocess", h.ReprocessDocument)
		biz.DELETE("/documents/:id", h.DeleteDocument)

		biz.GET("/tasks/:id", h.GetTask)
		biz.GET("/tasks/:id/trace", h.GetTaskTrace)
		biz.GET("/tasks/:id/feedback", h.ListTaskFeedback)
		biz.PUT("/tasks/:id/review", h.ReviewAffected)
		biz.PUT("/tasks/:id/generate", h.GenerateCases)
		biz.POST("/tasks/:id/retry", h.RetryTask)
		biz.GET("/jobs", h.ListJobs)
		biz.POST("/jobs/:id/retry", h.RetryJob)
		biz.POST("/jobs/:id/cancel", h.CancelJob)
		biz.POST("/jobs/:id/replay", h.ReplayJob)
		biz.GET("/workflows", h.ListWorkflows)

		biz.GET("/knowledge-suggestions", h.ListKnowledgeSuggestions)
		biz.POST("/knowledge-suggestions", h.CreateKnowledgeSuggestion)
		biz.POST("/knowledge-suggestions/:id/draft", h.DraftKnowledgeSuggestion)
		biz.PUT("/knowledge-suggestions/:id", h.UpdateKnowledgeSuggestion)

		knowledge := biz.Group("/knowledge")
		{
			knowledge.POST("", h.UploadKnowledge)
			knowledge.GET("", h.ListKnowledge)
			knowledge.GET("/:id", h.GetKnowledge)
			knowledge.POST("/:id/reprocess", h.ReprocessKnowledge)
			knowledge.PUT("/:id", h.UpdateKnowledge)
			knowledge.DELETE("/:id", h.DeleteKnowledge)
		}

		retrieval := biz.Group("/retrieval")
		{
			retrieval.POST("/documents", h.SearchDocuments)
			retrieval.POST("/knowledge", h.SearchKnowledge)
		}

		maintenance := biz.Group("/maintenance")
		{
			maintenance.GET("/vector-health", h.GetVectorHealth)
			maintenance.GET("/stale-index", h.ListStaleIndex)
			maintenance.POST("/reindex", h.ReindexVectors)
		}

		cases := biz.Group("/tasks/:id/cases")
		{
			cases.GET("", h.ListTestCases)
			cases.PUT("/:case_id", h.UpdateTestCase)
			cases.PUT("/:case_id/submit", h.SubmitTestCase)
			cases.POST("/:case_id/feedback", h.CreateCaseFeedback)
		}
	}

	return r
}
