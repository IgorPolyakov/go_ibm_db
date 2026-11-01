package stress_test

import (
	"context"
	"testing"

	ibmdb "github.com/ibmdb/go_ibm_db"
)

// TestGetInfoDirect isolates operation 10: GetInfo on a fresh connection
// and after a query on the same connection.
func TestGetInfoDirect(t *testing.T) {
	cfg := config()
	if cfg.dsn == "" {
		t.Skip("set DB2_STRESS_DSN to run against a live database")
	}
	ctx := context.Background()
	db, err := openDB(ctx, cfg.dsn, 2)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = db.Close() }()

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("conn: %v", err)
	}
	defer func() { _ = conn.Close() }()

	err = conn.Raw(func(raw any) error {
		c, ok := raw.(*ibmdb.Conn)
		if !ok {
			t.Fatalf("unexpected driver connection %T", raw)
		}
		name, err := c.GetInfo(17 /*SQL_DBMS_NAME*/)
		t.Logf("fresh: GetInfo=%q err=%v", name, err)
		if err != nil {
			return err
		}
		if name == "" {
			t.Error("empty SQL_DBMS_NAME on fresh connection")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	var n int64
	if err := conn.QueryRowContext(ctx, cfg.query).Scan(&n); err != nil {
		t.Fatalf("query: %v", err)
	}

	err = conn.Raw(func(raw any) error {
		c := raw.(*ibmdb.Conn)
		name, err := c.GetInfo(17)
		t.Logf("after query: GetInfo=%q err=%v", name, err)
		if err != nil {
			return err
		}
		if name == "" {
			t.Error("empty SQL_DBMS_NAME after a query")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
