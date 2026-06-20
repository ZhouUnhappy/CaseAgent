package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"caseagent/internal/db/models"

	"github.com/gin-gonic/gin"
)

type CreateTenantRequest struct {
	Slug string `json:"slug" binding:"required"`
	Name string `json:"name" binding:"required"`
}

type UpdateTenantRequest struct {
	Name string `json:"name" binding:"required"`
}

func (h *Handler) CreateTenant(c *gin.Context) {
	var req CreateTenantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.Slug = strings.TrimSpace(req.Slug)
	req.Name = strings.TrimSpace(req.Name)
	if req.Slug == "" || req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "slug and name are required"})
		return
	}

	tenant := &models.Tenant{
		Slug:      req.Slug,
		Name:      req.Name,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if _, err := DBFromContext(c).NewInsert().Model(tenant).Exec(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, tenant)
}

func (h *Handler) ListTenants(c *gin.Context) {
	tenants := []models.Tenant{}
	query := DBFromContext(c).NewSelect().Model(&tenants)
	if c.Query("include_archived") != "true" {
		query.Where("archived_at IS NULL")
	}
	if err := query.OrderExpr("archived_at ASC NULLS FIRST").Order("created_at DESC").Scan(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, tenants)
}

func (h *Handler) UpdateTenant(c *gin.Context) {
	tenant, ok := h.loadTenantBySlug(c)
	if !ok {
		return
	}
	var req UpdateTenantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	now := time.Now()
	if _, err := DBFromContext(c).NewUpdate().
		Model((*models.Tenant)(nil)).
		Set("name = ?", req.Name).
		Set("updated_at = ?", now).
		Where("id = ?", tenant.ID).
		Exec(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	tenant.Name = req.Name
	tenant.UpdatedAt = now
	c.JSON(http.StatusOK, tenant)
}

func (h *Handler) ArchiveTenant(c *gin.Context) {
	tenant, ok := h.loadTenantBySlug(c)
	if !ok {
		return
	}
	now := time.Now()
	if _, err := DBFromContext(c).NewUpdate().
		Model((*models.Tenant)(nil)).
		Set("archived_at = ?", now).
		Set("updated_at = ?", now).
		Where("id = ?", tenant.ID).
		Exec(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	tenant.ArchivedAt = &now
	tenant.UpdatedAt = now
	c.JSON(http.StatusOK, tenant)
}

func (h *Handler) UnarchiveTenant(c *gin.Context) {
	tenant, ok := h.loadTenantBySlug(c)
	if !ok {
		return
	}
	now := time.Now()
	if _, err := DBFromContext(c).NewUpdate().
		Model((*models.Tenant)(nil)).
		Set("archived_at = NULL").
		Set("updated_at = ?", now).
		Where("id = ?", tenant.ID).
		Exec(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	tenant.ArchivedAt = nil
	tenant.UpdatedAt = now
	c.JSON(http.StatusOK, tenant)
}

func (h *Handler) loadTenantBySlug(c *gin.Context) (*models.Tenant, bool) {
	slug := strings.TrimSpace(c.Param("slug"))
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant slug is required"})
		return nil, false
	}
	tenant := new(models.Tenant)
	if err := DBFromContext(c).NewSelect().Model(tenant).Where("slug = ?", slug).Scan(c.Request.Context()); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Tenant not found"})
			return nil, false
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return nil, false
	}
	return tenant, true
}
