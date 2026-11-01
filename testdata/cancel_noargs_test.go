package main

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestQueryContextCancelNoArgs verifies that an argument-free query (which
// database/sql routes through the driver's Queryer/QueryerContext fast path
// with SQLExecDirect) is aborted when its context times out while blocked
// on a row lock held by another connection.
func TestQueryContextCancelNoArgs(t *testing.T) {
	SkipOnPlatform(t, PlatformZOS)

	db := Createconnection()
	defer db.Close()

	db.Exec("DROP TABLE gocancel2")
	if _, err := db.Exec("CREATE TABLE gocancel2 (id INTEGER NOT NULL PRIMARY KEY, v INTEGER)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	defer db.Exec("DROP TABLE gocancel2")
	if _, err := db.Exec("INSERT INTO gocancel2 VALUES (1, 10)"); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// connection 1: hold a row lock inside a transaction
	c1, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("conn1: %v", err)
	}
	defer c1.Close()
	tx, err := c1.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	// an exclusive table lock makes even plain reads from c2 block
	if _, err := c1.ExecContext(context.Background(), "LOCK TABLE gocancel2 IN EXCLUSIVE MODE"); err != nil {
		t.Fatalf("lock table: %v", err)
	}

	// connection 2: enable lock waiting (Informix), then run an args-free
	// query that blocks, with a short context timeout
	c2, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("conn2: %v", err)
	}
	defer c2.Close()
	_, _ = c2.ExecContext(context.Background(), "SET LOCK MODE TO WAIT")

	qctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	started := time.Now()
	rows, qerr := c2.QueryContext(qctx, "SELECT v FROM gocancel2 WHERE id = 1")
	elapsed := time.Since(started)
	if qerr == nil {
		rows.Close()
	}
	_ = tx.Rollback()

	// the query must return shortly after the deadline, not stay blocked
	// until the lock is released (the reviewer's original complaint).
	// Depending on the server, the canceled call either fails or — as
	// Informix does after an aborted lock wait — returns committed data;
	// both prove the watcher aborted the blocked SQLExecDirect.
	if elapsed > 3*time.Second {
		t.Fatalf("canceled query took %v to return: cancel did not abort it", elapsed)
	}
	if elapsed < 400*time.Millisecond {
		t.Fatalf("query finished after only %v (err=%v): the lock did not block it", elapsed, qerr)
	}

	fmt.Printf("args-free blocked query returned after %v (err=%v)\n", elapsed, qerr)
}
