package db

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	"github.com/uptrace/bun"
)

var vectorTypePattern = regexp.MustCompile(`^vector\((\d+)\)$`)

type vectorColumnSpec struct {
	tableName  string
	columnName string
	indexName  string
}

var vectorColumns = []vectorColumnSpec{
	{tableName: "document_chunks", columnName: "embedding", indexName: "document_chunks_embedding_idx"},
	{tableName: "knowledge_base", columnName: "embedding", indexName: "knowledge_base_embedding_idx"},
}

func ensureVectorDimensions(ctx context.Context, db *bun.DB, dimensions int) error {
	if dimensions <= 0 {
		return nil
	}

	for _, spec := range vectorColumns {
		currentType, err := currentVectorType(ctx, db, spec.tableName, spec.columnName)
		if err != nil {
			return err
		}

		expectedType := fmt.Sprintf("vector(%d)", dimensions)
		if currentType == expectedType {
			continue
		}

		currentDimensions, err := parseVectorDimensions(currentType)
		if err != nil {
			return fmt.Errorf("inspect %s.%s: %w", spec.tableName, spec.columnName, err)
		}

		nonNullCount, err := countNonNullEmbeddings(ctx, db, spec.tableName, spec.columnName)
		if err != nil {
			return err
		}
		if nonNullCount > 0 {
			return fmt.Errorf(
				"vector dimension mismatch for %s.%s: database=%d config=%d and %d embeddings already exist; clear or re-embed the stored vectors before switching dimensions",
				spec.tableName,
				spec.columnName,
				currentDimensions,
				dimensions,
				nonNullCount,
			)
		}

		if err := alterVectorColumn(ctx, db, spec, dimensions); err != nil {
			return err
		}
	}

	return nil
}

func currentVectorType(ctx context.Context, db *bun.DB, tableName string, columnName string) (string, error) {
	var currentType string
	err := db.NewRaw(`
		SELECT format_type(a.atttypid, a.atttypmod)
		FROM pg_attribute AS a
		JOIN pg_class AS c ON c.oid = a.attrelid
		JOIN pg_namespace AS n ON n.oid = c.relnamespace
		WHERE c.relname = ?
		  AND a.attname = ?
		  AND a.attnum > 0
		  AND NOT a.attisdropped
		  AND n.nspname = current_schema()
	`, tableName, columnName).Scan(ctx, &currentType)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("column not found")
		}
		return "", fmt.Errorf("query current vector type: %w", err)
	}

	return currentType, nil
}

func countNonNullEmbeddings(ctx context.Context, db *bun.DB, tableName string, columnName string) (int, error) {
	query := fmt.Sprintf(
		`SELECT count(*) FROM %s WHERE %s IS NOT NULL`,
		quoteIdentifier(tableName),
		quoteIdentifier(columnName),
	)

	var count int
	if err := db.NewRaw(query).Scan(ctx, &count); err != nil {
		return 0, fmt.Errorf("count non-null embeddings for %s.%s: %w", tableName, columnName, err)
	}

	return count, nil
}

func alterVectorColumn(ctx context.Context, db *bun.DB, spec vectorColumnSpec, dimensions int) error {
	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		dropIndexSQL := fmt.Sprintf(`DROP INDEX IF EXISTS %s`, quoteIdentifier(spec.indexName))
		if _, err := tx.ExecContext(ctx, dropIndexSQL); err != nil {
			return fmt.Errorf("drop index %s: %w", spec.indexName, err)
		}

		alterColumnSQL := fmt.Sprintf(
			`ALTER TABLE %s ALTER COLUMN %s TYPE vector(%d)`,
			quoteIdentifier(spec.tableName),
			quoteIdentifier(spec.columnName),
			dimensions,
		)
		if _, err := tx.ExecContext(ctx, alterColumnSQL); err != nil {
			return fmt.Errorf("alter vector column %s.%s: %w", spec.tableName, spec.columnName, err)
		}

		createIndexSQL := fmt.Sprintf(
			`CREATE INDEX IF NOT EXISTS %s ON %s USING ivfflat (%s vector_cosine_ops)`,
			quoteIdentifier(spec.indexName),
			quoteIdentifier(spec.tableName),
			quoteIdentifier(spec.columnName),
		)
		if _, err := tx.ExecContext(ctx, createIndexSQL); err != nil {
			return fmt.Errorf("create index %s: %w", spec.indexName, err)
		}

		return nil
	})
}

func parseVectorDimensions(value string) (int, error) {
	matches := vectorTypePattern.FindStringSubmatch(strings.TrimSpace(value))
	if len(matches) != 2 {
		return 0, fmt.Errorf("unexpected vector type %q", value)
	}

	var dimensions int
	if _, err := fmt.Sscanf(matches[1], "%d", &dimensions); err != nil {
		return 0, fmt.Errorf("parse vector dimensions from %q: %w", value, err)
	}

	return dimensions, nil
}

func quoteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
