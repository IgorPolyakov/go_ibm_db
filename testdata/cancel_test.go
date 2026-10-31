package main

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestExecContextCancelDuringBlockedExec verifies that canceling the
// context aborts a statement that is blocked on a row lock held by another
// connection: the context watcher calls SQLCancel and the blocked
// ExecContext returns a context error promptly instead of hanging until
// the other transaction ends.
func TestExecContextCancelDuringBlockedExec(t *testing.T) {
	SkipOnPlatform(t, PlatformZOS) // lock-wait behavior differs on z/OS

	db := Createconnection()
	defer db.Close()

	db.Exec("DROP TABLE gocancel")
	if _, err := db.Exec("CREATE TABLE gocancel (id INTEGER NOT NULL PRIMARY KEY, v INTEGER)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	defer db.Exec("DROP TABLE gocancel")
	if _, err := db.Exec("INSERT INTO gocancel VALUES (1, 10)"); err != nil {
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
	if _, err := c1.ExecContext(context.Background(), "UPDATE gocancel SET v = v + 1 WHERE id = 1"); err != nil {
		t.Fatalf("update (lock): %v", err)
	}

	// connection 2: make the conflicting update block, then cancel it
	c2, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("conn2: %v", err)
	}
	defer c2.Close()
	// best effort: Informix returns an immediate lock error unless
	// lock-wait mode is enabled; on DB2 the update blocks by default
	_, _ = c2.ExecContext(context.Background(), "SET LOCK MODE TO WAIT")

	qctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	started := time.Now()
	_, eerr := c2.ExecContext(qctx, "UPDATE gocancel SET v = 99 WHERE id = 1")
	elapsed := time.Since(started)
	_ = tx.Rollback()

	if eerr == nil {
		t.Fatalf("expected the blocked statement to fail, it succeeded in %v", elapsed)
	}
	// the cancel must abort the blocked call promptly (slack for the
	// SQLCancel round-trip), not wait out the server lock timeout
	if elapsed > 8*time.Second {
		t.Fatalf("canceled statement took %v to return: SQLCancel did not abort it", elapsed)
	}
	// it must genuinely have blocked first, otherwise this did not test
	// cancellation of an in-flight call
	if elapsed < 2*time.Second {
		t.Fatalf("statement failed after only %v (%v): the row lock did not block it", elapsed, eerr)
	}

	fmt.Printf("blocked statement canceled after %v: %v\n", elapsed, eerr)
}
