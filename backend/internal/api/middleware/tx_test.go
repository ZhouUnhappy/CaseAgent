package middleware

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

func TestTxBuffersSuccessUntilCommit(t *testing.T) {
	state := newTxTestState(false)
	router := txTestRouter(t, state)
	router.GET("/ok", func(c *gin.Context) {
		AfterCommit(c, func() {
			state.afterCommit.Add(1)
		})
		c.JSON(http.StatusCreated, gin.H{"ok": true})
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ok", nil))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("body = %s, want success JSON", rec.Body.String())
	}
	if state.commits.Load() != 1 || state.rollbacks.Load() != 0 {
		t.Fatalf("commits=%d rollbacks=%d, want 1/0", state.commits.Load(), state.rollbacks.Load())
	}
	if state.afterCommit.Load() != 1 {
		t.Fatalf("afterCommit = %d, want 1", state.afterCommit.Load())
	}
}

func TestTxRollsBackAndFlushesHandlerError(t *testing.T) {
	state := newTxTestState(false)
	router := txTestRouter(t, state)
	router.GET("/bad", func(c *gin.Context) {
		AfterCommit(c, func() {
			state.afterCommit.Add(1)
		})
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad input"})
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/bad", nil))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "bad input") {
		t.Fatalf("body = %s, want handler error JSON", rec.Body.String())
	}
	if state.commits.Load() != 0 || state.rollbacks.Load() != 1 {
		t.Fatalf("commits=%d rollbacks=%d, want 0/1", state.commits.Load(), state.rollbacks.Load())
	}
	if state.afterCommit.Load() != 0 {
		t.Fatalf("afterCommit = %d, want 0", state.afterCommit.Load())
	}
}

func TestTxCommitFailureReturnsServerError(t *testing.T) {
	state := newTxTestState(true)
	router := txTestRouter(t, state)
	router.GET("/ok", func(c *gin.Context) {
		AfterCommit(c, func() {
			state.afterCommit.Add(1)
		})
		c.JSON(http.StatusCreated, gin.H{"ok": true})
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ok", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("body leaked buffered success response: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "request transaction failed") {
		t.Fatalf("body = %s, want transaction failure JSON", rec.Body.String())
	}
	if state.commits.Load() != 1 || state.rollbacks.Load() != 0 {
		t.Fatalf("commits=%d rollbacks=%d, want 1/0", state.commits.Load(), state.rollbacks.Load())
	}
	if state.afterCommit.Load() != 0 {
		t.Fatalf("afterCommit = %d, want 0", state.afterCommit.Load())
	}
}

func txTestRouter(t *testing.T, state *txTestState) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	sqldb := sql.OpenDB(&txTestConnector{state: state})
	t.Cleanup(func() {
		_ = sqldb.Close()
	})
	bunDB := bun.NewDB(sqldb, pgdialect.New())
	t.Cleanup(func() {
		_ = bunDB.Close()
	})

	router := gin.New()
	router.Use(Tx(bunDB))
	return router
}

type txTestState struct {
	commitErr   bool
	commits     atomic.Int64
	rollbacks   atomic.Int64
	afterCommit atomic.Int64
}

func newTxTestState(commitErr bool) *txTestState {
	return &txTestState{commitErr: commitErr}
}

type txTestConnector struct {
	state *txTestState
}

func (c *txTestConnector) Connect(context.Context) (driver.Conn, error) {
	return &txTestConn{state: c.state}, nil
}

func (c *txTestConnector) Driver() driver.Driver {
	return txTestDriver{}
}

type txTestDriver struct{}

func (txTestDriver) Open(string) (driver.Conn, error) {
	return &txTestConn{state: newTxTestState(false)}, nil
}

type txTestConn struct {
	state *txTestState
}

func (c *txTestConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare not implemented")
}

func (c *txTestConn) Close() error {
	return nil
}

func (c *txTestConn) Begin() (driver.Tx, error) {
	return &txTestTx{state: c.state}, nil
}

func (c *txTestConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return &txTestTx{state: c.state}, nil
}

type txTestTx struct {
	state *txTestState
}

func (tx *txTestTx) Commit() error {
	tx.state.commits.Add(1)
	if tx.state.commitErr {
		return errors.New("forced commit failure")
	}
	return nil
}

func (tx *txTestTx) Rollback() error {
	tx.state.rollbacks.Add(1)
	return nil
}
