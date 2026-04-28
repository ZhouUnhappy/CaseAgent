package db

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/uptrace/bun"
)

const schemaFileName = "001_init.sql"

func applySchema(ctx context.Context, db *bun.DB) error {
	schemaSQL, err := loadSchemaSQL()
	if err != nil {
		return err
	}

	if _, err := db.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("apply schema %s: %w", schemaFileName, err)
	}

	return nil
}

func loadSchemaSQL() (string, error) {
	path, err := schemaFilePath()
	if err != nil {
		return "", err
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read schema file %s: %w", path, err)
	}

	schema := strings.TrimSpace(string(content))
	if schema == "" {
		return "", fmt.Errorf("schema file %s is empty", path)
	}

	return schema, nil
}

func schemaFilePath() (string, error) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("resolve schema file path: runtime caller unavailable")
	}

	return filepath.Join(filepath.Dir(filename), "..", "..", "migrations", schemaFileName), nil
}
