package handler

import (
	"net/http"
	"time"

	"caseagent/internal/db/models"

	"github.com/gin-gonic/gin"
)

type CreateTenantRequest struct {
	Slug string `json:"slug" binding:"required"`
	Name string `json:"name" binding:"required"`
}

func (h *Handler) CreateTenant(c *gin.Context) {
	var req CreateTenantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tenant := &models.Tenant{
		Slug:      req.Slug,
		Name:      req.Name,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if _, err := DBFromContext(c).NewInsert().Model(tenant).Exec(c); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, tenant)
}

func (h *Handler) ListTenants(c *gin.Context) {
	tenants := []models.Tenant{}
	if err := DBFromContext(c).NewSelect().Model(&tenants).Order("created_at DESC").Scan(c); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, tenants)
}
