package stress_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	ibmdb "github.com/ibmdb/go_ibm_db"
)

// TestInformixCompat covers common Informix database operations.
// Each table belongs to one connection and is temporary.
func TestInformixCompat(t *testing.T) {
	cfg := config()
	if cfg.dsn == "" || os.Getenv("DB2_STRESS_PLATFORM") != "INFORMIX" {
		t.Skip("set DB2_STRESS_DSN and DB2_STRESS_PLATFORM=INFORMIX")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	db, err := openDB(ctx, cfg.dsn, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	var databaseName string
	if err = conn.QueryRowContext(ctx, "SELECT DBINFO('dbname') FROM systables WHERE tabid = 1").Scan(&databaseName); err != nil {
		t.Fatalf("current database: %v", err)
	}
	if _, err = conn.ExecContext(ctx, "DATABASE \""+strings.ReplaceAll(strings.TrimSpace(databaseName), "\"", "\"\"")+"\""); err != nil {
		t.Fatalf("switch database: %v", err)
	}

	table := fmt.Sprintf("gibc_%d", nextTable.Add(1))
	_, err = conn.ExecContext(ctx, "CREATE TEMP TABLE "+table+" (id SERIAL, name VARCHAR(80), payload BYTE, note TEXT)")
	if err != nil {
		t.Fatalf("create temp table: %v", err)
	}
	defer func() { _, _ = conn.ExecContext(context.Background(), "DROP TABLE "+table) }()

	for _, level := range []string{"DIRTY READ", "REPEATABLE READ", "CURSOR STABILITY", "COMMITTED READ"} {
		if _, err = conn.ExecContext(ctx, "SET ISOLATION TO "+level); err != nil {
			t.Fatalf("set isolation %s: %v", level, err)
		}
	}
	for _, mode := range []string{"NOT WAIT", "WAIT", "WAIT 2"} {
		if _, err = conn.ExecContext(ctx, "SET LOCK MODE TO "+mode); err != nil {
			t.Fatalf("set lock mode %s: %v", mode, err)
		}
	}

	insert, err := conn.PrepareContext(ctx, "INSERT INTO "+table+" (name, payload, note) VALUES (?, ?, ?)")
	if err != nil {
		t.Fatalf("prepare insert: %v", err)
	}
	defer func() { _ = insert.Close() }()
	payload := bytes.Repeat([]byte{0, 1, 2, 255}, 512*1024)
	noteValue := strings.Repeat("text", 512*1024)
	first, err := insert.ExecContext(ctx, "first", payload, noteValue)
	if err != nil {
		t.Fatalf("insert binary/text: %v", err)
	}
	firstID, err := first.LastInsertId()
	if err != nil || firstID <= 0 {
		t.Fatalf("first serial = %d, %v", firstID, err)
	}
	second, err := insert.ExecContext(ctx, "second", nil, nil)
	if err != nil {
		t.Fatalf("reuse prepared insert: %v", err)
	}
	secondID, err := second.LastInsertId()
	if err != nil || secondID <= firstID {
		t.Fatalf("second serial = %d, first = %d, err = %v", secondID, firstID, err)
	}
	if stable, err := first.LastInsertId(); err != nil || stable != firstID {
		t.Fatalf("first serial changed after second insert: %d, %v", stable, err)
	}
	plainTable := table + "_plain"
	if _, err = conn.ExecContext(ctx, "CREATE TEMP TABLE "+plainTable+" (id INTEGER)"); err != nil {
		t.Fatalf("create non-serial table: %v", err)
	}
	defer func() { _, _ = conn.ExecContext(context.Background(), "DROP TABLE "+plainTable) }()
	plain, err := conn.ExecContext(ctx, "INSERT INTO "+plainTable+" VALUES (?)", 42)
	if err != nil {
		t.Fatalf("insert without serial: %v", err)
	}
	if id, err := plain.LastInsertId(); err != nil || id != 0 {
		t.Fatalf("non-serial insert ID = %d, %v", id, err)
	}
	if _, err = conn.ExecContext(ctx, "CREATE UNIQUE INDEX "+table+"_name ON "+table+" (name)"); err != nil {
		t.Fatalf("create unique index: %v", err)
	}
	var duplicate *ibmdb.Error
	if _, err = insert.ExecContext(ctx, "first", nil, nil); !errors.As(err, &duplicate) || len(duplicate.Diag) == 0 || duplicate.Diag[0].NativeError != -239 {
		t.Fatalf("duplicate Informix error unavailable: %v", err)
	}

	var name, note string
	var gotPayload []byte
	if err = conn.QueryRowContext(ctx, "SELECT name, payload, note FROM "+table+" WHERE id = ?", firstID).Scan(&name, &gotPayload, &note); err != nil {
		t.Fatalf("read binary/text: %v", err)
	}
	if name != "first" || note != noteValue || !bytes.Equal(gotPayload, payload) {
		t.Fatalf("binary/text mismatch: name=%q note=%d bytes payload=%d bytes", name, len(note), len(gotPayload))
	}
	var nullPayload, nullNote []byte
	if err = conn.QueryRowContext(ctx, "SELECT payload, note FROM "+table+" WHERE id = ?", secondID).Scan(&nullPayload, &nullNote); err != nil {
		t.Fatalf("read NULL: %v", err)
	}
	if nullPayload != nil || nullNote != nil {
		t.Fatalf("NULL mismatch: payload=%v note=%v", nullPayload, nullNote)
	}
	rows, err := conn.QueryContext(ctx, "SELECT id, payload FROM "+table+" ORDER BY id")
	if err != nil {
		t.Fatalf("query mixed columns: %v", err)
	}
	defer func() { _ = rows.Close() }()
	for _, wantID := range []int64{firstID, secondID} {
		if !rows.Next() {
			t.Fatalf("missing row %d: %v", wantID, rows.Err())
		}
		var id int64
		var data []byte
		if err = rows.Scan(&id, &data); err != nil || id != wantID {
			t.Fatalf("mixed columns: id=%d want=%d err=%v", id, wantID, err)
		}
		if id == firstID && !bytes.Equal(data, payload) || id == secondID && data != nil {
			t.Fatalf("mixed columns: id=%d payload=%d bytes", id, len(data))
		}
	}
	if rows.Next() || rows.Err() != nil {
		t.Fatalf("unexpected extra row: %v", rows.Err())
	}
	if err = rows.Close(); err != nil {
		t.Fatalf("close mixed columns: %v", err)
	}

	var pagedID int64
	if err = conn.QueryRowContext(ctx, "SELECT SKIP ? FIRST ? id FROM "+table+" ORDER BY id", 1, 1).Scan(&pagedID); err != nil || pagedID != secondID {
		t.Fatalf("FIRST/SKIP = %d, want %d: %v", pagedID, secondID, err)
	}

	if _, err = conn.ExecContext(ctx, "UPDATE "+table+" SET name = ? WHERE id = ?", "updated", firstID); err != nil {
		t.Fatalf("update: %v", err)
	}
	if _, err = conn.ExecContext(ctx, "DELETE FROM "+table+" WHERE id = ?", secondID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	var diag *ibmdb.Error
	if _, err = conn.ExecContext(ctx, "SELECT * FROM gibc_missing_table"); !errors.As(err, &diag) || len(diag.Diag) == 0 || diag.Diag[0].NativeError != -206 {
		t.Fatalf("native Informix error unavailable: %v", err)
	}

	var count int64
	if err = conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil || count != 1 {
		t.Fatalf("connection after SQL error: count=%d err=%v", count, err)
	}
}

// TestInformixTransaction checks data visibility, not just Commit/Rollback return values.
func TestInformixTransaction(t *testing.T) {
	cfg := config()
	if cfg.dsn == "" || os.Getenv("DB2_STRESS_PLATFORM") != "INFORMIX" {
		t.Skip("set DB2_STRESS_DSN and DB2_STRESS_PLATFORM=INFORMIX")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	db, err := openDB(ctx, cfg.dsn, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	table := fmt.Sprintf("gibt_%d", nextTable.Add(1))
	if _, err = conn.ExecContext(ctx, "CREATE TEMP TABLE "+table+" (id INTEGER)"); err != nil {
		t.Fatalf("create logged temp table: %v", err)
	}
	defer func() { _, _ = conn.ExecContext(context.Background(), "DROP TABLE "+table) }()

	for _, commit := range []bool{false, true} {
		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO "+table+" (id) VALUES (?)", 1); err != nil {
			_ = tx.Rollback()
			t.Fatalf("insert in transaction: %v", err)
		}
		if commit {
			err = tx.Commit()
		} else {
			err = tx.Rollback()
		}
		if err != nil {
			t.Fatalf("end transaction: %v", err)
		}
		var count int64
		if err = conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
			t.Fatalf("count after transaction: %v", err)
		}
		want := int64(0)
		if commit {
			want = 1
		}
		if count != want {
			t.Fatalf("count after commit=%t: %d, want %d", commit, count, want)
		}
	}
}

func TestInformixScalarTypes(t *testing.T) {
	cfg := config()
	if cfg.dsn == "" || os.Getenv("DB2_STRESS_PLATFORM") != "INFORMIX" {
		t.Skip("set DB2_STRESS_DSN and DB2_STRESS_PLATFORM=INFORMIX")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	db, err := openDB(ctx, cfg.dsn, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	table := fmt.Sprintf("gibs_%d", nextTable.Add(1))
	if _, err = conn.ExecContext(ctx, "CREATE TEMP TABLE "+table+" (id INTEGER, amount DECIMAL(16,6), happened DATETIME YEAR TO SECOND)"); err != nil {
		t.Fatalf("create temp table: %v", err)
	}
	defer func() { _, _ = conn.ExecContext(context.Background(), "DROP TABLE "+table) }()
	happened := time.Date(2024, 2, 29, 23, 59, 59, 0, time.UTC)
	if _, err = conn.ExecContext(ctx, "INSERT INTO "+table+" VALUES (?, ?, ?)", 7, "1234567890.123456", happened); err != nil {
		t.Fatalf("insert scalars: %v", err)
	}
	var id int64
	var amount string
	var gotTime time.Time
	if err = conn.QueryRowContext(ctx, "SELECT id, amount, happened FROM "+table).Scan(&id, &amount, &gotTime); err != nil {
		t.Fatalf("read scalars: %v", err)
	}
	if id != 7 || amount != "1234567890.123456" || !gotTime.Equal(happened) {
		t.Fatalf("scalar mismatch: id=%d amount=%q time=%s", id, amount, gotTime)
	}
}

func TestInformixResultAfterConnectionClose(t *testing.T) {
	cfg := config()
	if cfg.dsn == "" || os.Getenv("DB2_STRESS_PLATFORM") != "INFORMIX" {
		t.Skip("set DB2_STRESS_DSN and DB2_STRESS_PLATFORM=INFORMIX")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	db, err := openDB(ctx, cfg.dsn, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxIdleConns(0)
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	table := fmt.Sprintf("gibr_%d", nextTable.Add(1))
	if _, err = conn.ExecContext(ctx, "CREATE TEMP TABLE "+table+" (id SERIAL)"); err != nil {
		_ = conn.Close()
		t.Fatal(err)
	}
	result, err := conn.ExecContext(ctx, "INSERT INTO "+table+" VALUES (0)")
	closeErr := conn.Close()
	if err != nil || closeErr != nil {
		t.Fatalf("insert=%v close=%v", err, closeErr)
	}
	id, err := result.LastInsertId()
	if err != nil || id <= 0 {
		t.Fatalf("serial after connection close = %d, %v", id, err)
	}
}
