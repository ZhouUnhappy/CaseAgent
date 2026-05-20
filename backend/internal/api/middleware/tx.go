package middleware

import (
	"context"
	"errors"
	"log/slog"

	"caseagent/internal/db"

	"github.com/gin-gonic/gin"
	"github.com/uptrace/bun"
)

// Tx wraps each request in a transaction. If the request carries a tenant
// (set by Tenant), the tx is RLS-scoped via SET LOCAL app.tenant_id; otherwise
// a plain tx opens (used by /tenants endpoints that bypass Tenant).
//
// The tx is stashed in gin.Context under "db" and reachable via
// handler.DBFromContext. Handlers returning status >= 400 trigger rollback.
//
// Caveat: gin flushes the response as c.JSON runs, so a commit failure
// happens after the client already saw success. We log it but cannot
// retroactively turn the response into a 5xx. Acceptable for the
// validation-phase project; production would need a buffered ResponseWriter.
func Tx(bunDB *bun.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		runHandler := func(ctx context.Context, tx bun.Tx) error {
			c.Set("db", tx)
			c.Request = c.Request.WithContext(ctx)
			c.Next()
			if c.Writer.Status() >= 400 {
				return errAbortTx
			}
			return nil
		}

		var err error
		if tid, ok := c.Get("tenant_id"); ok {
			ctx := db.WithTenant(c.Request.Context(), tid.(int))
			err = db.RunInTenantTx(ctx, bunDB, runHandler)
		} else {
			err = bunDB.RunInTx(c.Request.Context(), nil, runHandler)
		}
		if err != nil && !errors.Is(err, errAbortTx) {
			slog.Warn("request tx error", "path", c.Request.URL.Path, "error", err)
		}
	}
}

var errAbortTx = errors.New("handler returned error status, rolling back tx")
