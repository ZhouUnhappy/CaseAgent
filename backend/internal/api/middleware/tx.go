package middleware

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"

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
func Tx(bunDB *bun.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var afterCommitHooks []func()
		c.Set(afterCommitHooksKey, &afterCommitHooks)

		originalWriter := c.Writer
		bufferedWriter := newBufferedResponseWriter(originalWriter)
		c.Writer = bufferedWriter
		defer func() {
			c.Writer = originalWriter
		}()

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
			originalWriter.Header().Set("Content-Type", "application/json; charset=utf-8")
			originalWriter.WriteHeader(http.StatusInternalServerError)
			_, _ = originalWriter.Write([]byte(`{"error":"request transaction failed"}`))
			return
		}
		if err == nil {
			for _, hook := range afterCommitHooks {
				hook()
			}
		}
		bufferedWriter.FlushTo(originalWriter)
	}
}

var errAbortTx = errors.New("handler returned error status, rolling back tx")

const afterCommitHooksKey = "after_commit_hooks"

// AfterCommit queues work to run only after the request transaction commits.
// It is meant for background jobs that read rows inserted or updated by the
// request handler.
func AfterCommit(c *gin.Context, hook func()) {
	if hook == nil {
		return
	}
	value, ok := c.Get(afterCommitHooksKey)
	if !ok {
		hook()
		return
	}
	hooks, ok := value.(*[]func())
	if !ok {
		hook()
		return
	}
	*hooks = append(*hooks, hook)
}

type bufferedResponseWriter struct {
	gin.ResponseWriter
	header     http.Header
	body       bytes.Buffer
	status     int
	size       int
	wrote      bool
	wroteFinal bool
}

func newBufferedResponseWriter(w gin.ResponseWriter) *bufferedResponseWriter {
	header := make(http.Header, len(w.Header()))
	for key, values := range w.Header() {
		header[key] = append([]string{}, values...)
	}
	return &bufferedResponseWriter{
		ResponseWriter: w,
		header:         header,
		status:         http.StatusOK,
		size:           -1,
	}
}

func (w *bufferedResponseWriter) Header() http.Header {
	return w.header
}

func (w *bufferedResponseWriter) WriteHeader(statusCode int) {
	if w.wrote {
		return
	}
	w.status = statusCode
	w.wrote = true
	w.size = 0
}

func (w *bufferedResponseWriter) WriteHeaderNow() {
	if !w.wrote {
		w.WriteHeader(w.status)
	}
}

func (w *bufferedResponseWriter) Write(data []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.body.Write(data)
	w.size += n
	return n, err
}

func (w *bufferedResponseWriter) WriteString(value string) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.body.WriteString(value)
	w.size += n
	return n, err
}

func (w *bufferedResponseWriter) Status() int {
	return w.status
}

func (w *bufferedResponseWriter) Size() int {
	return w.size
}

func (w *bufferedResponseWriter) Written() bool {
	return w.wrote
}

func (w *bufferedResponseWriter) Flush() {
	w.WriteHeaderNow()
}

func (w *bufferedResponseWriter) FlushTo(dst gin.ResponseWriter) {
	if w.wroteFinal {
		return
	}
	w.wroteFinal = true
	for key := range dst.Header() {
		dst.Header().Del(key)
	}
	for key, values := range w.header {
		for _, value := range values {
			dst.Header().Add(key, value)
		}
	}
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	dst.WriteHeader(w.status)
	if w.body.Len() > 0 {
		_, _ = dst.Write(w.body.Bytes())
	}
}
