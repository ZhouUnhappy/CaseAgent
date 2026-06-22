package db

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/uptrace/bun"
)

const embeddingDimensionsPlaceholder = "{{EMBEDDING_DIMENSIONS}}"

type storedSchemaBaseline struct {
	SchemaHash          string `bun:"schema_hash"`
	EmbeddingDimensions int    `bun:"embedding_dimensions"`
}

func ensureSchemaBaseline(ctx context.Context, db *bun.DB, dimensions int) error {
	schemaSQL, err := loadSchemaSQL(schemaFilePath(), dimensions)
	if err != nil {
		return err
	}
	expectedHash := schemaHash(schemaSQL)

	exists, err := schemaBaselineExists(ctx, db)
	if err != nil {
		return err
	}
	if exists {
		stored, err := loadStoredSchemaBaseline(ctx, db)
		if err != nil {
			return err
		}
		if stored.SchemaHash != expectedHash || stored.EmbeddingDimensions != dimensions {
			return fmt.Errorf(
				"database schema baseline does not match this build; clear the database before restarting (database hash=%s dimensions=%d, expected hash=%s dimensions=%d)",
				stored.SchemaHash,
				stored.EmbeddingDimensions,
				expectedHash,
				dimensions,
			)
		}
		return nil
	}

	tableCount, err := currentSchemaTableCount(ctx, db)
	if err != nil {
		return err
	}
	if tableCount != 0 {
		return fmt.Errorf("database contains %d table(s) without the current schema baseline; clear the database before restarting", tableCount)
	}

	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.ExecContext(ctx, schemaSQL); err != nil {
			return fmt.Errorf("initialize schema baseline: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO caseagent_schema (singleton, schema_hash, embedding_dimensions)
			VALUES (TRUE, ?, ?)
		`, expectedHash, dimensions); err != nil {
			return fmt.Errorf("record schema baseline: %w", err)
		}
		return nil
	})
}

func loadSchemaSQL(path string, dimensions int) (string, error) {
	if dimensions <= 0 {
		return "", fmt.Errorf("embedding dimensions must be > 0")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read schema file %s: %w", path, err)
	}

	schema := strings.TrimSpace(string(content))
	if schema == "" {
		return "", fmt.Errorf("schema file %s is empty", path)
	}
	if count := strings.Count(schema, embeddingDimensionsPlaceholder); count != 2 {
		return "", fmt.Errorf("schema file %s contains %d embedding dimension placeholders, expected 2", path, count)
	}
	return strings.ReplaceAll(schema, embeddingDimensionsPlaceholder, fmt.Sprintf("%d", dimensions)), nil
}

func schemaFilePath() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	return filepath.Join(filepath.Dir(filename), "..", "..", "schema", "schema.sql")
}

func schemaBaselineExists(ctx context.Context, db bun.IDB) (bool, error) {
	var exists bool
	if err := db.NewRaw(`
		SELECT to_regclass(current_schema() || '.caseagent_schema') IS NOT NULL
	`).Scan(ctx, &exists); err != nil {
		return false, fmt.Errorf("inspect schema baseline: %w", err)
	}
	return exists, nil
}

func loadStoredSchemaBaseline(ctx context.Context, db bun.IDB) (storedSchemaBaseline, error) {
	var stored storedSchemaBaseline
	if err := db.NewRaw(`
		SELECT schema_hash, embedding_dimensions
		FROM caseagent_schema
		WHERE singleton = TRUE
	`).Scan(ctx, &stored); err != nil {
		return storedSchemaBaseline{}, fmt.Errorf("load schema baseline: %w", err)
	}
	return stored, nil
}

func currentSchemaTableCount(ctx context.Context, db bun.IDB) (int, error) {
	var count int
	if err := db.NewRaw(`
		SELECT count(*)
		FROM pg_tables
		WHERE schemaname = current_schema()
	`).Scan(ctx, &count); err != nil {
		return 0, fmt.Errorf("inspect current schema tables: %w", err)
	}
	return count, nil
}

func schemaHash(schemaSQL string) string {
	sum := sha256.Sum256([]byte(schemaSQL))
	return hex.EncodeToString(sum[:])
}
