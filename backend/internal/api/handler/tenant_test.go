package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"caseagent/internal/api/middleware"
	"caseagent/internal/db/models"

	"github.com/gin-gonic/gin"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

func TestTenantLifecycleHandlers(t *testing.T) {
	dsn := os.Getenv("CASEAGENT_TEST_DSN")
	if dsn == "" {
		t.Skip("set CASEAGENT_TEST_DSN to run tenant handler integration test")
	}

	ctx := context.Background()
	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
	t.Cleanup(func() { _ = sqldb.Close() })
	bunDB := bun.NewDB(sqldb, pgdialect.New())
	t.Cleanup(func() { _ = bunDB.Close() })

	slug := fmt.Sprintf("tenant-handler-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		_, _ = bunDB.NewDelete().Model((*models.Tenant)(nil)).Where("slug = ?", slug).Exec(ctx)
	})

	router := tenantTestRouter(bunDB)

	created := tenantRequest[models.Tenant](t, router, http.MethodPost, "/api/v1/tenants", map[string]string{
		"slug": " " + slug + " ",
		"name": " Tenant Handler ",
	}, http.StatusCreated)
	if created.Slug != slug || created.Name != "Tenant Handler" {
		t.Fatalf("created tenant = %#v, want trimmed slug/name", created)
	}

	updated := tenantRequest[models.Tenant](t, router, http.MethodPut, "/api/v1/tenants/"+slug, map[string]string{
		"name": " Renamed Tenant ",
	}, http.StatusOK)
	if updated.Name != "Renamed Tenant" {
		t.Fatalf("updated name = %q, want Renamed Tenant", updated.Name)
	}

	archived := tenantRequest[models.Tenant](t, router, http.MethodPost, "/api/v1/tenants/"+slug+"/archive", nil, http.StatusOK)
	if archived.ArchivedAt == nil {
		t.Fatal("archive response archived_at is nil")
	}

	active := tenantRequest[[]models.Tenant](t, router, http.MethodGet, "/api/v1/tenants", nil, http.StatusOK)
	if containsTenant(active, slug) {
		t.Fatalf("archived tenant %q leaked into active list", slug)
	}

	all := tenantRequest[[]models.Tenant](t, router, http.MethodGet, "/api/v1/tenants?include_archived=true", nil, http.StatusOK)
	if !containsTenant(all, slug) {
		t.Fatalf("archived tenant %q missing from include_archived list", slug)
	}

	restored := tenantRequest[models.Tenant](t, router, http.MethodPost, "/api/v1/tenants/"+slug+"/unarchive", nil, http.StatusOK)
	if restored.ArchivedAt != nil {
		t.Fatalf("unarchive response archived_at = %v, want nil", restored.ArchivedAt)
	}
}

func tenantTestRouter(db *bun.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := New(db)
	router := gin.New()
	tenants := router.Group("/api/v1/tenants")
	tenants.Use(middleware.Tx(db))
	tenants.POST("", h.CreateTenant)
	tenants.GET("", h.ListTenants)
	tenants.PUT("/:slug", h.UpdateTenant)
	tenants.POST("/:slug/archive", h.ArchiveTenant)
	tenants.POST("/:slug/unarchive", h.UnarchiveTenant)
	return router
}

func tenantRequest[T any](t *testing.T, router *gin.Engine, method string, path string, payload any, wantStatus int) T {
	t.Helper()
	var body *bytes.Reader
	if payload == nil {
		body = bytes.NewReader(nil)
	} else {
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		body = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("%s %s status = %d, want %d; body=%s", method, path, rec.Code, wantStatus, rec.Body.String())
	}
	var out T
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode %s %s response: %v; body=%s", method, path, err, rec.Body.String())
	}
	return out
}

func containsTenant(tenants []models.Tenant, slug string) bool {
	for _, tenant := range tenants {
		if strings.EqualFold(tenant.Slug, slug) {
			return true
		}
	}
	return false
}
