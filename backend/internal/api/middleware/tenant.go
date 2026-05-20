package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"caseagent/internal/db/models"

	"github.com/gin-gonic/gin"
	"github.com/uptrace/bun"
)

// Tenant resolves the X-Tenant-ID header (a tenant slug) to a tenants.id and
// stashes it in gin.Context under "tenant_id". Routes that don't need a
// tenant context (e.g. /api/v1/tenants CRUD) should not register this
// middleware.
func Tenant(bunDB *bun.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		slug := strings.TrimSpace(c.GetHeader("X-Tenant-ID"))
		if slug == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "missing X-Tenant-ID header"})
			return
		}

		tenant := &models.Tenant{}
		if err := bunDB.NewSelect().Model(tenant).Where("slug = ?", slug).Scan(c.Request.Context()); err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("tenant %q not found", slug)})
			return
		}

		c.Set("tenant_id", tenant.ID)
		c.Next()
	}
}
