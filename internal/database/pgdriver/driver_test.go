package pgdriver

import (
	"database/sql/driver"
	"testing"
	"time"
)

func TestInterpolateQuery(t *testing.T) {
	ts := time.Date(2026, 10, 17, 10, 0, 0, 0, time.UTC)
	args := []driver.NamedValue{
		{Ordinal: 1, Value: "Fanpage O'Reilly"},
		{Ordinal: 2, Value: int64(42)},
		{Ordinal: 3, Value: true},
		{Ordinal: 4, Value: ts},
		{Ordinal: 5, Value: nil},
	}

	q, err := InterpolateQuery("INSERT INTO pages (name, count, active, created_at, deleted_at) VALUES ($1, $2, $3, $4, $5)", args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "INSERT INTO pages (name, count, active, created_at, deleted_at) VALUES ('Fanpage O''Reilly', 42, TRUE, '2026-10-17T10:00:00Z', NULL)"
	if q != expected {
		t.Fatalf("expected:\n%s\ngot:\n%s", expected, q)
	}
}
