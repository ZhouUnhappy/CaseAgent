package db

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

type vectorColumnSpec struct {
	tableName  string
	columnName string
}

var vectorColumns = []vectorColumnSpec{
	{tableName: "document_chunks", columnName: "embedding"},
	{tableName: "knowledge_base", columnName: "embedding"},
}

func validateVectorDimensions(ctx context.Context, db *bun.DB, dimensions int) error {
	if dimensions <= 0 {
		return fmt.Errorf("embedding dimensions must be > 0")
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

		return fmt.Errorf(
			"vector dimension mismatch for %s.%s: database=%s config=%s; clear the database before restarting",
			spec.tableName,
			spec.columnName,
			currentType,
			expectedType,
		)
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
		return "", fmt.Errorf("query current vector type: %w", err)
	}

	return currentType, nil
}
