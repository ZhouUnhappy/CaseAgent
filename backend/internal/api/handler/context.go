package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/uptrace/bun"
)

// DBFromContext returns the tx-scoped IDB injected by middleware.Tx. Panics
// if Tx middleware was not applied to the route — that is a programming
// error (wiring bug), not a runtime condition.
func DBFromContext(c *gin.Context) bun.IDB {
	v, ok := c.Get("db")
	if !ok {
		panic("handler.DBFromContext: no db in context — middleware.Tx not applied?")
	}
	tx, ok := v.(bun.Tx)
	if !ok {
		panic("handler.DBFromContext: context db is not bun.Tx")
	}
	return tx
}

// TenantIDFromContext returns the tenant ID resolved by middleware.Tenant.
// Returns ok=false for routes that bypass Tenant (e.g. /tenants CRUD).
func TenantIDFromContext(c *gin.Context) (int, bool) {
	v, ok := c.Get("tenant_id")
	if !ok {
		return 0, false
	}
	id, ok := v.(int)
	return id, ok
}
