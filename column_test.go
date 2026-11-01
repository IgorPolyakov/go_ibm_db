package go_ibm_db

import (
	"testing"
	"time"
	"unsafe"

	"github.com/ibmdb/go_ibm_db/api"
)

// TestWideColumnValueNoPanic verifies that BaseColumn.Value accepts wide
// character buffers larger than 2 MiB. The old fixed *[1<<20]uint16 view
// panicked ("slice bounds out of range") on such values.
func TestWideColumnValueNoPanic(t *testing.T) {
	t.Parallel()

	for _, n := range []int{1 << 19, 1 << 20, (1 << 20) + 123} {
		us := make([]uint16, n)
		for i := range us {
			us[i] = uint16('a' + i%26)
		}
		buf := unsafe.Slice((*byte)(unsafe.Pointer(&us[0])), 2*n)

		c := &BaseColumn{CType: api.SQL_C_WCHAR}
		v, err := c.Value(buf)
		if err != nil {
			t.Fatalf("n=%d: Value: %v", n, err)
		}
		// utf16toutf8 returns the UTF-8 bytes; ASCII stays one byte per rune
		var got int
		switch val := v.(type) {
		case string:
			got = len(val)
		case []byte:
			got = len(val)
		default:
			t.Fatalf("n=%d: got %T, want string or []byte", n, v)
		}
		if got != n {
			t.Fatalf("n=%d: decoded %d units, want %d", n, got, n)
		}
	}
}

// TestPoolReuseResetsIdleTimer verifies that returning a reused pooled
// connection restarts its idle lifetime: a stale timer from an earlier
// return must not close the connection before the new lifetime elapses.
func TestPoolReuseResetsIdleTimer(t *testing.T) {
	t.Parallel()

	const lifetime = 700 * time.Millisecond
	p := Pconnect("PoolSize=1")
	p.mu.Lock()
	p.maxLifetime = lifetime
	p.mu.Unlock()

	dbp := p.Open("dsn")
	if dbp == nil {
		t.Fatal("Open returned nil")
	}
	dbp.Close() // first return: timer would fire at ~700ms (stale)

	time.Sleep(150 * time.Millisecond)
	dbp2 := p.Open("dsn")
	if dbp2 != dbp {
		if dbp2 != nil {
			dbp2.Close()
		}
		t.Fatal("expected to reuse the same pooled connection")
	}
	time.Sleep(50 * time.Millisecond)
	dbp2.Close() // second return: fresh timer fires at ~900ms mark

	// At ~450ms after the FIRST return (450ms after the second) the
	// connection must still be available: the stale first timer must not
	// have closed it.
	time.Sleep(250 * time.Millisecond)
	p.mu.Lock()
	avail := len(p.availablePool["dsn"])
	p.mu.Unlock()
	if avail != 1 {
		t.Fatalf("connection closed prematurely %v after the last return: stale idle timer fired", 300*time.Millisecond)
	}

	// After the full lifetime has elapsed since the second return, the
	// connection must be gone.
	time.Sleep(lifetime)
	p.mu.Lock()
	avail = len(p.availablePool["dsn"])
	p.mu.Unlock()
	if avail != 0 {
		t.Fatal("idle connection was not closed after its lifetime")
	}
}
