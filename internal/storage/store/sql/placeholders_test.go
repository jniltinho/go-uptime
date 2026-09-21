// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package sql

import (
	"database/sql/driver"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestTranslatePlaceholders(t *testing.T) {
	scenarios := []struct {
		name            string
		query           string
		expectedQuery   string
		expectedIndexes []int
		expectedCount   int
	}{
		{name: "none", query: "SELECT 1", expectedQuery: "SELECT 1", expectedCount: 0},
		{name: "in-order", query: "SELECT a FROM t WHERE b = $1 AND c = $2", expectedQuery: "SELECT a FROM t WHERE b = ? AND c = ?", expectedIndexes: []int{0, 1}, expectedCount: 2},
		{name: "repeated", query: "WHERE a >= $1 AND b >= $1 AND c = $2", expectedQuery: "WHERE a >= ? AND b >= ? AND c = ?", expectedIndexes: []int{0, 0, 1}, expectedCount: 2},
		{name: "reordered", query: "WHERE key IN ($2, $3) AND rn <= $1", expectedQuery: "WHERE key IN (?, ?) AND rn <= ?", expectedIndexes: []int{1, 2, 0}, expectedCount: 3},
		{name: "multi-digit", query: "VALUES ($10, $2)", expectedQuery: "VALUES (?, ?)", expectedIndexes: []int{9, 1}, expectedCount: 10},
		{name: "string-literal", query: "SELECT 'costs $1', $1", expectedQuery: "SELECT 'costs $1', ?", expectedIndexes: []int{0}, expectedCount: 1},
		{name: "escaped-quotes", query: `SELECT 'it''s $1', 'a\'$2', $1`, expectedQuery: `SELECT 'it''s $1', 'a\'$2', ?`, expectedIndexes: []int{0}, expectedCount: 1},
		{name: "double-quoted-identifier", query: `SELECT "condition", "col$1" FROM t WHERE a = $1`, expectedQuery: `SELECT "condition", "col$1" FROM t WHERE a = ?`, expectedIndexes: []int{0}, expectedCount: 1},
		{name: "backtick-identifier", query: "SELECT `a$1` FROM t WHERE a = $1", expectedQuery: "SELECT `a$1` FROM t WHERE a = ?", expectedIndexes: []int{0}, expectedCount: 1},
		{name: "line-comment", query: "SELECT $1 -- $2 ?\n, $2", expectedQuery: "SELECT ? -- $2 ?\n, ?", expectedIndexes: []int{0, 1}, expectedCount: 2},
		{name: "hash-comment", query: "SELECT $1 # $2 ?\n", expectedQuery: "SELECT ? # $2 ?\n", expectedIndexes: []int{0}, expectedCount: 1},
		{name: "block-comment", query: "SELECT /* $2 ? */ $1", expectedQuery: "SELECT /* $2 ? */ ?", expectedIndexes: []int{0}, expectedCount: 1},
		{name: "double-minus-without-space", query: "SELECT $1--$2", expectedQuery: "SELECT ?--?", expectedIndexes: []int{0, 1}, expectedCount: 2},
		{name: "dollar-without-digit", query: "SELECT '$' || $1, a$b", expectedQuery: "SELECT '$' || ?, a$b", expectedIndexes: []int{0}, expectedCount: 1},
		{name: "gap", query: "WHERE a = $1 AND b = $3", expectedQuery: "WHERE a = ? AND b = ?", expectedIndexes: []int{0, 2}, expectedCount: 3},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			translated, err := parsePlaceholders(scenario.query)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if translated.query != scenario.expectedQuery {
				t.Errorf("expected query %q, got %q", scenario.expectedQuery, translated.query)
			}
			if !reflect.DeepEqual(translated.argumentIndexes, scenario.expectedIndexes) {
				t.Errorf("expected indexes %v, got %v", scenario.expectedIndexes, translated.argumentIndexes)
			}
			if translated.numberOfArguments != scenario.expectedCount {
				t.Errorf("expected %d arguments, got %d", scenario.expectedCount, translated.numberOfArguments)
			}
		})
	}
}

func TestTranslatePlaceholders_InsertReturning(t *testing.T) {
	translated, err := parsePlaceholders("\n\t\tINSERT INTO endpoint_results (endpoint_id, success)\n\t\tVALUES ($1, $2)\n\t\tRETURNING endpoint_result_id\n\t")
	if err != nil {
		t.Fatal(err)
	}
	if translated.returningColumn != "endpoint_result_id" || translated.insertWithoutReturning != "INSERT INTO endpoint_results (endpoint_id, success)\n\t\tVALUES (?, ?)" {
		t.Errorf("unexpected emulation of RETURNING: column=%q insert=%q", translated.returningColumn, translated.insertWithoutReturning)
	}
	for _, query := range []string{
		"SELECT endpoint_id FROM endpoints WHERE endpoint_key = $1",
		"UPDATE endpoints SET endpoint_name = $1 RETURNING endpoint_id",
		"INSERT INTO endpoints (endpoint_key) VALUES ($1) RETURNING endpoint_id, endpoint_key",
	} {
		if translated, err := parsePlaceholders(query); err != nil || len(translated.returningColumn) > 0 {
			t.Errorf("%q: expected no RETURNING emulation, got %q (err=%v)", query, translated.returningColumn, err)
		}
	}
}

func TestTranslatePlaceholders_Invalid(t *testing.T) {
	for _, query := range []string{"SELECT ?", "SELECT $0", "SELECT $65536", "SELECT $1 WHERE a = ?"} {
		if _, err := parsePlaceholders(query); !errors.Is(err, errInvalidPlaceholder) {
			t.Errorf("%q: expected errInvalidPlaceholder, got %v", query, err)
		}
	}
}

func TestTranslatedQuery_Arguments(t *testing.T) {
	translated, err := translatePlaceholders("WHERE a >= $1 AND b >= $1 AND c = $2")
	if err != nil {
		t.Fatal(err)
	}
	arguments, err := translated.arguments([]driver.NamedValue{{Ordinal: 1, Value: int64(10)}, {Ordinal: 2, Value: "x"}})
	if err != nil {
		t.Fatal(err)
	}
	expected := []driver.NamedValue{{Ordinal: 1, Value: int64(10)}, {Ordinal: 2, Value: int64(10)}, {Ordinal: 3, Value: "x"}}
	if !reflect.DeepEqual(arguments, expected) {
		t.Errorf("expected %v, got %v", expected, arguments)
	}
	for _, invalid := range [][]driver.NamedValue{
		{{Ordinal: 1, Value: int64(10)}},
		{{Ordinal: 1, Value: int64(10)}, {Ordinal: 2, Value: "x"}, {Ordinal: 3, Value: "y"}},
	} {
		if _, err := translated.arguments(invalid); !errors.Is(err, errArgumentCountInvalid) {
			t.Errorf("expected errArgumentCountInvalid for %d arguments, got %v", len(invalid), err)
		}
	}
	if _, err := translated.arguments([]driver.NamedValue{{Name: "a", Ordinal: 1, Value: 1}, {Ordinal: 2, Value: 2}}); !errors.Is(err, errNamedArgument) {
		t.Errorf("expected errNamedArgument, got %v", err)
	}
}

func TestTranslatePlaceholders_Cache(t *testing.T) {
	first, err := translatePlaceholders("SELECT $1 -- cache")
	if err != nil {
		t.Fatal(err)
	}
	second, _ := translatePlaceholders("SELECT $1 -- cache")
	if first != second {
		t.Error("expected the translation to be cached")
	}
	for i := 0; i < maximumCachedTranslations+10; i++ {
		if _, err := translatePlaceholders("SELECT $1 -- " + strconv.Itoa(i)); err != nil {
			t.Fatal(err)
		}
	}
	translationsMutex.Lock()
	size := len(translations)
	translationsMutex.Unlock()
	if size > maximumCachedTranslations {
		t.Errorf("expected at most %d cached translations, got %d", maximumCachedTranslations, size)
	}
}

func BenchmarkTranslatePlaceholders(b *testing.B) {
	query := "SELECT endpoint_id, " + strings.Repeat("$1, ", 50) + "$2 FROM endpoints WHERE endpoint_key = $3"
	for i := 0; i < b.N; i++ {
		if _, err := translatePlaceholders(query); err != nil {
			b.Fatal(err)
		}
	}
}
