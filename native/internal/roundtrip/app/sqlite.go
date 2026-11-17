package main

import (
	"context"
	"errors"
	"reflect"

	"github.com/egoist/mygo/native/internal/roundtrip/check"
	"github.com/egoist/mygo/plugins/sqlite"
)

// checkSQLite checks the SQLite plugin's native module, which phones build
// from its Swift and its Kotlin as they do the round trip's.
func checkSQLite(t check.T) {
	ctx := context.Background()
	db, err := sqlite.Open(ctx, ":memory:", sqlite.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Execute(ctx, "CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT NOT NULL, score REAL, data BLOB)"); err != nil {
		t.Fatal(err)
	}
	r, err := db.Execute(ctx, "INSERT INTO t (name, score, data) VALUES (?, ?, ?)", "héllo 🌍", 0.5, []byte{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Changes != 1 || r.LastInsertID != 1 {
		t.Fatalf("insert: %+v", r)
	}
	rows, err := db.Query(ctx, "SELECT id, name, score, data, NULL AS none, sqlite_version() FROM t")
	if err != nil {
		t.Fatal(err)
	}
	want := sqlite.Rows{Columns: []string{"id", "name", "score", "data", "none", "sqlite_version()"}, Values: [][]any{{int64(1), "héllo 🌍", 0.5, []byte{}, nil, "3.53.4"}}}
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("query: %#v, want %#v", rows, want)
	}
	var e *sqlite.Error
	if _, err := db.Execute(ctx, "INSERT INTO t (name) VALUES (NULL)"); !errors.As(err, &e) || e.ExtendedCode != 1299 {
		t.Fatalf("NOT NULL: %v, want SQLITE_CONSTRAINT_NOTNULL", err)
	}
	if _, err := db.Execute(ctx, "SELEC 1"); !errors.As(err, &e) || e.Code != 1 {
		t.Fatalf("syntax: %v, want SQLITE_ERROR", err)
	}
	if _, err := db.Execute(ctx, "SELECT 1; SELECT 2"); err == nil {
		t.Fatal("two statements ran")
	}
	if _, err := db.Execute(ctx, "ATTACH ':memory:' AS other"); err == nil {
		t.Fatal("ATTACH ran")
	}
	results, err := db.Transaction(ctx, []sqlite.Statement{
		{SQL: "INSERT INTO t (name) VALUES (?)", Args: []any{"a"}},
		{SQL: "UPDATE t SET score = score + 1 WHERE score IS NOT NULL"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].LastInsertID != 2 || results[1].Changes != 1 {
		t.Fatalf("transaction: %+v", results)
	}
	if _, err := db.Transaction(ctx, []sqlite.Statement{{SQL: "INSERT INTO t (name) VALUES ('b')"}, {SQL: "INSERT INTO t (name) VALUES (NULL)"}}); err == nil {
		t.Fatal("a failing transaction committed")
	}
	if rows, err := db.Query(ctx, "SELECT count(*) FROM t"); err != nil || rows.Values[0][0] != int64(2) {
		t.Fatalf("after rollback: %v %v", rows, err)
	}
}
