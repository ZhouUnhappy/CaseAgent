package workflow

import (
	"context"
	"fmt"

	tenantdb "caseagent/internal/db"
	"caseagent/internal/db/models"

	"github.com/uptrace/bun"
)

type ModelCallRecorder interface {
	RecordModelCall(ctx context.Context, input ModelCallInput) (*models.ModelCall, error)
}

type AgentTraceRecorder interface {
	ModelCallRecorder
	StartAgentRun(ctx context.Context, input StartAgentRunInput) (*models.AgentRun, error)
	FinishAgentRun(ctx context.Context, agentRunID int, input FinishAgentRunInput) error
}

type Recorder struct {
	db     bun.IDB
	rootDB *bun.DB
}

func NewRecorder(db bun.IDB) *Recorder {
	return &Recorder{db: db}
}

func NewTenantRecorder(rootDB *bun.DB) *Recorder {
	return &Recorder{rootDB: rootDB}
}

func (r *Recorder) RecordAgentRun(ctx context.Context, input AgentRunInput) (*models.AgentRun, error) {
	var row *models.AgentRun
	err := r.withDB(ctx, func(ctx context.Context, db bun.IDB) error {
		var err error
		row, err = New(db).RecordAgentRun(ctx, input)
		return err
	})
	return row, err
}

func (r *Recorder) StartAgentRun(ctx context.Context, input StartAgentRunInput) (*models.AgentRun, error) {
	var row *models.AgentRun
	err := r.withDB(ctx, func(ctx context.Context, db bun.IDB) error {
		var err error
		row, err = New(db).StartAgentRun(ctx, input)
		return err
	})
	return row, err
}

func (r *Recorder) FinishAgentRun(ctx context.Context, agentRunID int, input FinishAgentRunInput) error {
	return r.withDB(ctx, func(ctx context.Context, db bun.IDB) error {
		return New(db).FinishAgentRun(ctx, agentRunID, input)
	})
}

func (r *Recorder) RecordRetrievalRun(ctx context.Context, input RetrievalRunInput) (*models.RetrievalRun, error) {
	var row *models.RetrievalRun
	err := r.withDB(ctx, func(ctx context.Context, db bun.IDB) error {
		var err error
		row, err = New(db).RecordRetrievalRun(ctx, input)
		return err
	})
	return row, err
}

func (r *Recorder) RecordModelCall(ctx context.Context, input ModelCallInput) (*models.ModelCall, error) {
	var row *models.ModelCall
	err := r.withDB(ctx, func(ctx context.Context, db bun.IDB) error {
		var err error
		row, err = New(db).RecordModelCall(ctx, input)
		return err
	})
	return row, err
}

func (r *Recorder) RecordArtifact(ctx context.Context, input ArtifactInput) (*models.Artifact, error) {
	var row *models.Artifact
	err := r.withDB(ctx, func(ctx context.Context, db bun.IDB) error {
		var err error
		row, err = New(db).RecordArtifact(ctx, input)
		return err
	})
	return row, err
}

func (r *Recorder) withDB(ctx context.Context, fn func(context.Context, bun.IDB) error) error {
	switch {
	case r == nil:
		return fmt.Errorf("workflow recorder is nil")
	case r.rootDB != nil:
		tenantID, ok := tenantdb.TenantFromContext(ctx)
		if !ok {
			return fmt.Errorf("workflow recorder: no tenant in context")
		}
		return tenantdb.RunInTenantTx(tenantdb.WithTenant(ctx, tenantID), r.rootDB, func(ctx context.Context, tx bun.Tx) error {
			return fn(ctx, tx)
		})
	case r.db != nil:
		return fn(ctx, r.db)
	default:
		return fmt.Errorf("workflow recorder has no database")
	}
}
