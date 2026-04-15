package router

import (
	"caseagent/internal/api/handler"

	"github.com/gin-gonic/gin"
)

func SetupRouter(h *handler.Handler) *gin.Engine {
	r := gin.Default()

	// API v1
	v1 := r.Group("/api/v1")
	{
		// Projects
		projects := v1.Group("/projects")
		{
			projects.POST("", h.CreateProject)
			projects.GET("", h.ListProjects)
			projects.GET("/:id", h.GetProject)
			projects.PUT("/:id", h.UpdateProject)
			projects.DELETE("/:id", h.DeleteProject)

			// Documents (nested under projects)
			projects.GET("/:id/documents", h.ListDocuments)
			projects.POST("/:id/documents", h.UploadDocument)

			// Tasks (nested under projects)
			projects.GET("/:id/tasks", h.ListTasks)
			projects.POST("/:id/tasks", h.CreateGenerationTask)
		}

		// Documents (standalone)
		v1.GET("/documents/:id", h.GetDocument)
		v1.DELETE("/documents/:id", h.DeleteDocument)

		// Tasks (standalone)
		v1.GET("/tasks/:id", h.GetTask)
		v1.PUT("/tasks/:id/review", h.ReviewAffected)
		v1.PUT("/tasks/:id/generate", h.GenerateCases)

		// Knowledge Base
		knowledge := v1.Group("/knowledge")
		{
			knowledge.POST("", h.UploadKnowledge)
			knowledge.GET("", h.ListKnowledge)
			knowledge.GET("/:id", h.GetKnowledge)
			knowledge.PUT("/:id", h.UpdateKnowledge)
			knowledge.DELETE("/:id", h.DeleteKnowledge)
		}

		// Test Cases
		cases := v1.Group("/tasks/:id/cases")
		{
			cases.GET("", h.ListTestCases)
			cases.PUT("/:case_id", h.UpdateTestCase)
			cases.PUT("/:case_id/submit", h.SubmitTestCase)
		}
	}

	return r
}
