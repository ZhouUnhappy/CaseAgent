package db

import (
	"context"
	"database/sql"
	"fmt"

	"caseagent/internal/config"
	"caseagent/internal/db/models"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	"github.com/uptrace/bun/extra/bundebug"
)

var DB *bun.DB

func Init(ctx context.Context) error {
	cfg := config.Get().Database

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		cfg.User,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		cfg.DBName,
		cfg.SSLMode,
	)

	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
	DB = bun.NewDB(sqldb, pgdialect.New())

	if config.Get().Server.Mode == "debug" {
		DB = DB.WithQueryHook(bundebug.NewQueryHook(
			bundebug.WithVerbose(true),
			bundebug.FromEnv("BUNDEBUG"),
		))
	}

	// Register models
	for _, model := range []interface{}{
		&models.Tenant{},
		&models.Project{},
		&models.Document{},
		&models.DocumentChunk{},
		&models.KnowledgeBase{},
		&models.TestCase{},
		&models.TestCaseFeedback{},
		&models.CaseGenerationTask{},
		&models.BackgroundJob{},
		&models.WorkflowRun{},
		&models.WorkflowStep{},
		&models.AgentRun{},
		&models.ModelCall{},
		&models.RetrievalRun{},
		&models.Artifact{},
		&models.KnowledgeUpdateSuggestionGroup{},
		&models.KnowledgeUpdateSuggestionOccurrence{},
	} {
		DB.RegisterModel(model)
	}

	if err := DB.PingContext(ctx); err != nil {
		return err
	}

	if err := applySchema(ctx, DB); err != nil {
		return err
	}

	if err := ensureVectorDimensions(ctx, DB, config.Get().Model.Embedding.Dimensions); err != nil {
		return err
	}
	if err := validateRuntimeConfig(ctx, DB, config.Get()); err != nil {
		return err
	}

	return nil
}

func Close() error {
	if DB != nil {
		return DB.Close()
	}
	return nil
}
