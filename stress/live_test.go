package stress_test

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ibmdb "github.com/ibmdb/go_ibm_db"
)

const defaultQuery = "SELECT 1 FROM SYSIBM.SYSDUMMY1"

type liveConfig struct {
	dsn              string
	query            string
	rowsQuery        string
	operationTimeout time.Duration
	createTable      string
	insertRow        string
	selectRow        string
	deleteRows       string
	dropTable        string
}

func config() liveConfig {
	query := os.Getenv("DB2_STRESS_QUERY")
	if query == "" {
		query = defaultQuery
	}
	rowsQuery := os.Getenv("DB2_STRESS_ROWS_QUERY")
	if rowsQuery == "" {
		rowsQuery = query
	}
	cfg := liveConfig{
		dsn:              os.Getenv("DB2_STRESS_DSN"),
		query:            query,
		rowsQuery:        rowsQuery,
		operationTimeout: 5 * time.Second,
	}
	if os.Getenv("DB2_STRESS_PLATFORM") == "INFORMIX" {
		cfg.createTable = "CREATE TEMP TABLE {table} (id INTEGER) WITH NO LOG"
		cfg.insertRow = "INSERT INTO {table} (id) VALUES (?)"
		cfg.selectRow = "SELECT id FROM {table} WHERE id = ?"
		cfg.deleteRows = "DELETE FROM {table}"
		cfg.dropTable = "DROP TABLE {table}"
	} else {
		cfg.createTable = "DECLARE GLOBAL TEMPORARY TABLE SESSION.{table} (id INTEGER) ON COMMIT PRESERVE ROWS NOT LOGGED"
		cfg.insertRow = "INSERT INTO SESSION.{table} (id) VALUES (?)"
		cfg.selectRow = "SELECT id FROM SESSION.{table} WHERE id = ?"
		cfg.deleteRows = "DELETE FROM SESSION.{table}"
		cfg.dropTable = "DROP TABLE SESSION.{table}"
	}
	return cfg
}

func openDB(ctx context.Context, dsn string, maxOpen int) (*sql.DB, error) {
	db, err := sql.Open("go_ibm_db", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxOpen)
	if err = db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func TestLiveStress(t *testing.T) {
	cfg := config()
	if cfg.dsn == "" {
		t.Skip("set DB2_STRESS_DSN to run against a live database")
	}
	duration := stressDuration(t, "DB2_STRESS_DURATION", 30*time.Second)
	workers := stressInt(t, "DB2_STRESS_WORKERS", 8, 1, 64)
	seed := stressSeed(t)
	setupCtx, stopSetup := context.WithTimeout(context.Background(), time.Minute)
	defer stopSetup()

	dbs := make([]*sql.DB, 2)
	var err error
	dbs[0], err = openDB(setupCtx, cfg.dsn, workers*2+2)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = dbs[0].Close() }()
	dbs[1], err = openDB(setupCtx, cfg.dsn+";FETCHSIZE=8", workers*2+2)
	if err != nil {
		t.Fatalf("connect with array fetch: %v", err)
	}
	defer func() { _ = dbs[1].Close() }()

	pool := ibmdb.Pconnect(fmt.Sprintf("PoolSize=%d", workers))
	pool.SetConnMaxLifetime(30)
	if !pool.Init(1, cfg.dsn) {
		t.Fatal("initialize custom pool")
	}
	defer pool.Release()
	pool.Display() //nolint:staticcheck // SA1019: intentionally covers the deprecated debug helper
	sessions := make([]*liveSession, workers)
	for worker := range workers {
		operationCtx, stop := context.WithTimeout(setupCtx, cfg.operationTimeout)
		sessions[worker], err = openSession(operationCtx, dbs[worker%len(dbs)], cfg)
		stop()
		if err != nil {
			t.Fatalf("worker=%d setup: %v", worker, err)
		}
		defer sessions[worker].Close(cfg)
	}
	for worker, session := range sessions {
		for operation := byte(0); operation < operationCount; operation++ {
			operationCtx, stop := context.WithTimeout(setupCtx, cfg.operationTimeout)
			err = runOperation(operationCtx, dbs[worker%len(dbs)], pool, session, cfg, operation)
			stop()
			if err != nil {
				t.Fatalf("worker=%d initial operation=%d seed=%d: %v", worker, operation, seed, err)
			}
		}
	}
	before := takeLeakSnapshot()

	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	var count atomic.Uint64
	failures := make(chan error, 1)
	var group sync.WaitGroup
	for worker := range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			rng := rand.New(rand.NewSource(seed + int64(worker)))
			db := dbs[worker%len(dbs)]
			session := sessions[worker]
			for iteration := 0; ctx.Err() == nil; iteration++ {
				operation := byte(rng.Intn(operationCount))
				operationCtx, stop := context.WithTimeout(ctx, cfg.operationTimeout)
				err := runOperation(operationCtx, db, pool, session, cfg, operation)
				stop()
				if ctx.Err() != nil {
					return
				}
				if err != nil {
					reportFailure(failures, cancel, fmt.Errorf("worker=%d iteration=%d operation=%d seed=%d: %w", worker, iteration, operation, seed, err))
					return
				}
				count.Add(1)
			}
		}()
	}
	// progress ticker for soak analysis: cumulative ops every 15s
	tickerDone := make(chan struct{})
	ticker := time.NewTicker(15 * time.Second)
	lastCount := uint64(0)
	lastTime := time.Now()
	go func() {
		defer close(tickerDone)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				now := uint64(0)
				total := count.Load()
				now, lastCount = total-lastCount, total
				elapsed := time.Since(lastTime).Seconds()
				lastTime = time.Now()
				t.Logf("progress: ops=%d rate=%.0f/s", total, float64(now)/elapsed)
			case <-ctx.Done():
				return
			}
		}
	}()

	group.Wait()
	ticker.Stop()
	<-tickerDone
	// database/sql releases statements and connections asynchronously
	// (connection cleaner goroutine); let it settle before comparing
	time.Sleep(2 * time.Second)
	assertNoLeaks(t, before, takeLeakSnapshot(), workers)
	select {
	case err := <-failures:
		t.Fatal(err)
	default:
	}
	if count.Load() == 0 {
		t.Fatal("no operations completed")
	}
	t.Logf("seed=%d workers=%d duration=%s operations=%d", seed, workers, duration, count.Load())
}

func FuzzLiveSequence(f *testing.F) {
	sequence := make([]byte, operationCount)
	for i := range sequence {
		sequence[i] = byte(i)
	}
	f.Add(sequence)
	f.Add([]byte{7, 8, 5, 6, 12, 13})
	cfg := config()
	if cfg.dsn == "" {
		f.Skip("set DB2_STRESS_DSN to fuzz a live database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := openDB(ctx, cfg.dsn+";FETCHSIZE=8", 16)
	if err != nil {
		f.Fatalf("connect: %v", err)
	}
	f.Cleanup(func() { _ = db.Close() })
	pool := ibmdb.Pconnect("PoolSize=4")
	f.Cleanup(pool.Release)
	f.Fuzz(func(t *testing.T, sequence []byte) {
		if len(sequence) == 0 || len(sequence) > 32 {
			t.Skip()
		}
		setupCtx, stop := context.WithTimeout(context.Background(), cfg.operationTimeout)
		session, err := openSession(setupCtx, db, cfg)
		stop()
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		defer session.Close(cfg)
		for index, operation := range sequence {
			operationCtx, stop := context.WithTimeout(context.Background(), cfg.operationTimeout)
			err := runOperation(operationCtx, db, pool, session, cfg, operation)
			stop()
			if err != nil {
				t.Fatalf("operation=%d index=%d sequence=%x: %v", operation%operationCount, index, sequence, err)
			}
		}
	})
}

func reportFailure(failures chan<- error, cancel context.CancelFunc, err error) {
	select {
	case failures <- err:
	default:
	}
	cancel()
}

func stressDuration(t *testing.T, key string, fallback time.Duration) time.Duration {
	t.Helper()
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		t.Fatalf("invalid %s: %q", key, value)
	}
	return duration
}

func stressInt(t *testing.T, key string, fallback, min, max int) int {
	t.Helper()
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < min || n > max {
		t.Fatalf("invalid %s: expected %d..%d", key, min, max)
	}
	return n
}

func stressSeed(t *testing.T) int64 {
	t.Helper()
	value := os.Getenv("DB2_STRESS_SEED")
	if value == "" {
		return 1
	}
	seed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		t.Fatalf("invalid DB2_STRESS_SEED: %q", value)
	}
	return seed
}

// procFDCount returns the number of open file descriptors (linux only).
func procFDCount() int {
	if runtime.GOOS != "linux" {
		return -1
	}
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return -1
	}
	return len(entries)
}

// procRSSKB returns the resident set size in KiB (linux only).
func procRSSKB() int {
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return -1
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "VmRSS:") {
			kb := 0
			fmt.Sscanf(line, "VmRSS: %d kB", &kb) //nolint:errcheck
			return kb
		}
	}
	return -1
}

type leakSnapshot struct {
	env, conn, stmt int
	fds             int
	rssKB           int
}

func takeLeakSnapshot() leakSnapshot {
	env, conn, stmt := ibmdb.HandleCounts()
	return leakSnapshot{env: env, conn: conn, stmt: stmt, fds: procFDCount(), rssKB: procRSSKB()}
}

// assertNoLeaks compares snapshots taken before and after the load: every
// statement and environment handle must be released, and the descriptor
// count must not creep. Connection handles may legitimately remain held by
// the database/sql pool, so they are reported but not asserted.
func assertNoLeaks(t *testing.T, before, after leakSnapshot, workers int) {
	t.Helper()
	if after.stmt != before.stmt {
		t.Errorf("leak: statement handles %d -> %d during the load", before.stmt, after.stmt)
	}
	if after.env != before.env {
		t.Errorf("leak: environment handles %d -> %d during the load", before.env, after.env)
	}
	if after.fds >= 0 && before.fds >= 0 && after.fds-before.fds > workers*2+16 {
		t.Errorf("leak: file descriptors %d -> %d during the load", before.fds, after.fds)
	}
	t.Logf("handles: env %d->%d conn %d->%d stmt %d->%d; fds %d->%d; rss %d->%d kB",
		before.env, after.env, before.conn, after.conn, before.stmt, after.stmt,
		before.fds, after.fds, before.rssKB, after.rssKB)
}
