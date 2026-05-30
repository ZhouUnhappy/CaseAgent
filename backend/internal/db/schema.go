package db

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/uptrace/bun"
)

const schemaFilesPattern = "*.sql"

func applySchema(ctx context.Context, db *bun.DB) error {
	files, err := schemaFilePaths()
	if err != nil {
		return err
	}

	for _, file := range files {
		schemaSQL, err := loadSchemaSQL(file)
		if err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, schemaSQL); err != nil {
			return fmt.Errorf("apply schema %s: %w", filepath.Base(file), err)
		}
	}

	return nil
}

func loadSchemaSQL(path string) (string, error) {
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

func schemaFilePaths() ([]string, error) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return nil, fmt.Errorf("resolve schema file path: runtime caller unavailable")
	}

	pattern := filepath.Join(filepath.Dir(filename), "..", "..", "migrations", schemaFilesPattern)
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob schema files %s: %w", pattern, err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no schema files match %s", pattern)
	}
	sort.Strings(files)
	return files, nil
}
