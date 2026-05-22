package middleware_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"caseagent/internal/api/middleware"
	"caseagent/internal/db/models"

	"github.com/gin-gonic/gin"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

// TestTenantMiddleware verifies the X-Tenant-ID header is resolved to a
// tenants.id and stashed in gin.Context, and that missing/unknown values
// abort with 400. Gated on CASEAGENT_TEST_DSN because the resolver issues
// a real SELECT against the tenants table — the middleware has no mocking
// seam and bun's query path is what's under test.
func TestTenantMiddleware(t *testing.T) {
	dsn := os.Getenv("CASEAGENT_TEST_DSN")
	if dsn == "" {
		t.Skip("set CASEAGENT_TEST_DSN to run tenant middleware integration test")
	}

	ctx := context.Background()
	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
	t.Cleanup(func() { _ = sqldb.Close() })
	bunDB := bun.NewDB(sqldb, pgdialect.New())

	tenant := &models.Tenant{Slug: "mw-test", Name: "Middleware Test"}
	if _, err := bunDB.NewInsert().Model(tenant).Returning("id").Exec(ctx); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	t.Cleanup(func() {
		_, _ = bunDB.NewDelete().Model((*models.Tenant)(nil)).Where("id = ?", tenant.ID).Exec(ctx)
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.Tenant(bunDB))
	router.GET("/probe", func(c *gin.Context) {
		tid, ok := c.Get("tenant_id")
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant_id missing"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"tenant_id": tid})
	})

	cases := []struct {
		name           string
		header         string
		setHeader      bool
		wantStatus     int
		wantTenantID   int
		wantErrSubstr  string
	}{
		{
			name:         "valid slug stashes tenant_id",
			header:       "mw-test",
			setHeader:    true,
			wantStatus:   http.StatusOK,
			wantTenantID: tenant.ID,
		},
		{
			name:          "missing header rejected",
			setHeader:     false,
			wantStatus:    http.StatusBadRequest,
			wantErrSubstr: "missing X-Tenant-ID header",
		},
		{
			name:          "whitespace-only header rejected",
			header:        "   ",
			setHeader:     true,
			wantStatus:    http.StatusBadRequest,
			wantErrSubstr: "missing X-Tenant-ID header",
		},
		{
			name:          "unknown slug rejected",
			header:        "does-not-exist",
			setHeader:     true,
			wantStatus:    http.StatusBadRequest,
			wantErrSubstr: `tenant "does-not-exist" not found`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/probe", nil)
			if tc.setHeader {
				req.Header.Set("X-Tenant-ID", tc.header)
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status: got %d, want %d (body=%s)", rec.Code, tc.wantStatus, rec.Body.String())
			}

			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body: %v (raw=%s)", err, rec.Body.String())
			}

			if tc.wantErrSubstr != "" {
				errMsg, _ := body["error"].(string)
				if !strings.Contains(errMsg, tc.wantErrSubstr) {
					t.Fatalf("error: got %q, want substring %q", errMsg, tc.wantErrSubstr)
				}
			}

			if tc.wantTenantID != 0 {
				gotID, ok := body["tenant_id"].(float64)
				if !ok {
					t.Fatalf("tenant_id missing or not numeric in body: %s", rec.Body.String())
				}
				if int(gotID) != tc.wantTenantID {
					t.Fatalf("tenant_id: got %d, want %d", int(gotID), tc.wantTenantID)
				}
			}
		})
	}
}
