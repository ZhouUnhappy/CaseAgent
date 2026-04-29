package handler

import (
	"net/http"

	retrievalservice "caseagent/internal/service/retrieval"

	"github.com/gin-gonic/gin"
)

type SearchDocumentsRequest struct {
	Query       string   `json:"query"`
	Queries     []string `json:"queries"`
	TopK        int      `json:"top_k"`
	DocumentIDs []int    `json:"document_ids"`
}

type SearchKnowledgeRequest struct {
	Query string `json:"query" binding:"required"`
	TopK  int    `json:"top_k"`
	Type  string `json:"type"`
}

func (h *Handler) SearchDocuments(c *gin.Context) {
	var req SearchDocumentsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	svc := retrievalservice.New(h.DB)
	queries := append([]string{}, req.Queries...)
	if req.Query != "" {
		queries = append([]string{req.Query}, queries...)
	}
	if len(queries) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query or queries is required"})
		return
	}

	var (
		results []retrievalservice.DocumentResult
		err     error
	)
	if len(queries) == 1 {
		results, err = svc.SearchDocuments(c, queries[0], req.TopK, req.DocumentIDs)
	} else {
		results, err = svc.SearchDocumentsMultiQuery(c, queries, req.TopK, req.DocumentIDs)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"items": results,
	})
}

func (h *Handler) SearchKnowledge(c *gin.Context) {
	var req SearchKnowledgeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	svc := retrievalservice.New(h.DB)
	results, err := svc.SearchKnowledge(c, req.Query, req.TopK, req.Type)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"items": results,
	})
}
