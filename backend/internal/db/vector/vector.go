package vector

import (
	"database/sql/driver"
	"fmt"
	"strconv"
	"strings"
)

// Vector encodes pgvector values using the textual representation expected by PostgreSQL.
type Vector []float32

func New(values []float32) Vector {
	if len(values) == 0 {
		return nil
	}

	result := make(Vector, len(values))
	copy(result, values)
	return result
}

func (v Vector) Value() (driver.Value, error) {
	if len(v) == 0 {
		return nil, nil
	}

	var builder strings.Builder
	builder.Grow(len(v)*12 + 2)
	builder.WriteByte('[')
	for i, value := range v {
		if i > 0 {
			builder.WriteByte(',')
		}
		builder.WriteString(strconv.FormatFloat(float64(value), 'f', -1, 32))
	}
	builder.WriteByte(']')

	return builder.String(), nil
}

func (v *Vector) Scan(src any) error {
	if src == nil {
		*v = nil
		return nil
	}

	switch value := src.(type) {
	case string:
		return v.parse(value)
	case []byte:
		return v.parse(string(value))
	default:
		return fmt.Errorf("unsupported vector source type %T", src)
	}
}

func (v *Vector) parse(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		*v = nil
		return nil
	}
	if len(raw) < 2 || raw[0] != '[' || raw[len(raw)-1] != ']' {
		return fmt.Errorf("invalid vector format %q", raw)
	}

	body := strings.TrimSpace(raw[1 : len(raw)-1])
	if body == "" {
		*v = Vector{}
		return nil
	}

	parts := strings.Split(body, ",")
	parsed := make(Vector, 0, len(parts))
	for _, part := range parts {
		number, err := strconv.ParseFloat(strings.TrimSpace(part), 32)
		if err != nil {
			return fmt.Errorf("parse vector element %q: %w", part, err)
		}
		parsed = append(parsed, float32(number))
	}

	*v = parsed
	return nil
}
