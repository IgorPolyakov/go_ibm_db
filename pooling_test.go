package go_ibm_db

import (
	"database/sql"
	"sync"
	"testing"
	"time"
)

// TestPoolSize verifies size() sums used and available connections for a pool.
func TestPoolSize(t *testing.T) {
	t.Parallel()

	p := Pconnect("PoolSize=3")

	p.usedPool["dsn1"] = []*DBP{{}, {}}
	p.availablePool["dsn2"] = []*DBP{{}}

	if got := p.size(); got != 3 {
		t.Errorf("expected size 3, got %d", got)
	}
}

// TestPoolCapacityIndependence verifies that filling one pool to capacity
// does not affect another independently created pool's capacity accounting.
func TestPoolCapacityIndependence(t *testing.T) {
	t.Parallel()

	poolA := Pconnect("PoolSize=1")
	poolB := Pconnect("PoolSize=1")

	poolA.usedPool["dsn"] = []*DBP{{}}

	poolA.mu.Lock()
	aFull := poolA.size() >= poolA.poolSize
	poolA.mu.Unlock()
	if !aFull {
		t.Fatalf("expected poolA to be at capacity")
	}

	poolB.mu.Lock()
	bUnderCapacity := poolB.size() < poolB.poolSize
	poolB.mu.Unlock()
	if !bUnderCapacity {
		t.Errorf("poolB capacity should be unaffected by poolA reaching its limit")
	}
}

func TestPoolOpenDoesNotReturnConnectionTwice(t *testing.T) {
	t.Parallel()

	p := Pconnect("PoolSize=32")
	if !p.Init(1, "dsn") {
		t.Fatal("expected pool initialization to succeed")
	}

	results := make(chan *DBP, 32)
	var wg sync.WaitGroup
	for range cap(results) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- p.Open("dsn")
		}()
	}
	wg.Wait()
	close(results)

	seen := make(map[*DBP]bool)
	for db := range results {
		if db == nil {
			continue
		}
		if seen[db] {
			t.Fatalf("pool returned connection %p more than once", db)
		}
		seen[db] = true
	}
	if len(seen) > p.poolSize {
		t.Fatalf("pool returned %d connections for capacity %d", len(seen), p.poolSize)
	}
}

func TestPoolLifetimeIsIndependent(t *testing.T) {
	t.Parallel()

	poolA := Pconnect("PoolSize=1")
	poolB := Pconnect("PoolSize=1")

	poolA.SetConnMaxLifetime(1)

	poolA.mu.Lock()
	lifetimeA := poolA.maxLifetime
	poolA.mu.Unlock()
	poolB.mu.Lock()
	lifetimeB := poolB.maxLifetime
	poolB.mu.Unlock()

	if lifetimeA != time.Second {
		t.Fatalf("expected poolA lifetime of one second, got %s", lifetimeA)
	}
	if lifetimeB != time.Duration(defaultConnMaxLifetime)*time.Second {
		t.Fatalf("expected poolB to retain the default lifetime, got %s", lifetimeB)
	}
}

func TestPoolReleasePreventsNewConnections(t *testing.T) {
	t.Parallel()

	p := Pconnect("PoolSize=1")
	p.Release()

	if got := p.Open("dsn"); got != nil {
		t.Fatal("expected Open to return nil after Release")
	}
	if p.Init(1, "dsn") {
		t.Fatal("expected Init to fail after Release")
	}
}

// TestPconnectInvalidParamsNoPanic verifies that malformed pool option
// strings produce a usable pool instead of a panic.
func TestPconnectInvalidParamsNoPanic(t *testing.T) {
	t.Parallel()

	for _, s := range []string{"PoolSize", "PoolSize=", "PoolSize=abc", "PoolSize=0", "Foo=1", ""} {
		p := Pconnect(s)
		if p == nil {
			t.Fatalf("Pconnect(%q) returned nil", s)
		}
		if p.poolSize != defaultMaxIdleConns && s == "PoolSize=0" {
			t.Fatalf("Pconnect(%q): poolSize = %d, want default %d", s, p.poolSize, defaultMaxIdleConns)
		}
	}
}

// TestPoolOpenAfterReleaseWhileWaiting verifies that Open returns nil
// instead of panicking when Release runs while Open waits for a free slot.
func TestPoolOpenAfterReleaseWhileWaiting(t *testing.T) {
	t.Parallel()

	p := Pconnect("PoolSize=1")
	// occupy the only slot with a lazily-opened handle
	filled, err := sql.Open("go_ibm_db", "dsn")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	p.usedPool["dsn"] = []*DBP{{DB: filled, con: "dsn", pool: p}}

	p.mu.Lock()
	p.maxLifetime = 2 * time.Second
	p.mu.Unlock()

	done := make(chan *DBP, 1)
	go func() { done <- p.Open("dsn") }()

	time.Sleep(300 * time.Millisecond)
	p.Release() // must not panic the waiting Open

	select {
	case dbp := <-done:
		if dbp != nil {
			t.Fatal("expected Open to return nil after Release")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Open did not return after Release")
	}
}
