package go_ibm_db

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	trc "github.com/ibmdb/go_ibm_db/log2"
)

// DBP struct type contains the timeout, dbinstance pointer and connection string
// DBP wraps a *sql.DB with its connection string, lifetime and owning pool.
type DBP struct {
	*sql.DB
	con       string
	n         time.Duration
	pool      *Pool       // owning pool; avoids relying on the shared global pool pointer
	idleTimer *time.Timer // idle lifetime timer; guarded by pool.mu
	idleGen   uint64      // invalidates stale timer callbacks; guarded by pool.mu
}

// Pool struct contains the about the pool like size, used and available connections
// Pool is a connection pool keyed by connection string.
type Pool struct {
	availablePool map[string][]*DBP
	usedPool      map[string][]*DBP
	poolSize      int
	maxLifetime   time.Duration
	closed        bool
	mu            sync.Mutex
}

const (
	defaultMaxIdleConns    = 2
	defaultConnMaxLifetime = 60
)

// Pconnect returns a pool instance for the given option string.
func Pconnect(poolSize string) *Pool {
	trc.Trace1("pooling.go: Pconnect() - ENTRY")
	trc.Trace1("poolSize=" + poolSize)

	var size int
	count := len(poolSize)
	if count > 0 {
		opt := strings.SplitN(poolSize, "=", 2)
		if len(opt) == 2 && opt[0] == "PoolSize" {
			n, err := strconv.Atoi(opt[1])
			if err != nil || n <= 0 {
				size = defaultMaxIdleConns
			} else {
				size = n
			}
		} else {
			fmt.Println("Not a valid parameter")
		}
	} else {
		size = defaultMaxIdleConns
	}
	p := &Pool{
		availablePool: make(map[string][]*DBP),
		usedPool:      make(map[string][]*DBP),
		poolSize:      size,
		maxLifetime:   time.Duration(defaultConnMaxLifetime) * time.Second,
	}

	trc.Trace1("pooling.go: Pconnect() - EXIT")
	return p
}

// size returns the total number of connections (used + available) currently
// owned by this pool. Callers must hold p.mu.
func (p *Pool) size() int {
	total := 0
	for _, v := range p.usedPool {
		total += len(v)
	}
	for _, v := range p.availablePool {
		total += len(v)
	}
	return total
}

func (p *Pool) takeAvailableLocked(connStr string, lifetime time.Duration) *DBP {
	val, ok := p.availablePool[connStr]
	if !ok || len(val) == 0 {
		return nil
	}

	dbpo := val[0]
	if len(val) == 1 {
		delete(p.availablePool, connStr)
	} else {
		copy(val[0:], val[1:])
		val[len(val)-1] = nil
		p.availablePool[connStr] = val[:len(val)-1]
	}
	p.usedPool[connStr] = append(p.usedPool[connStr], dbpo)
	dbpo.SetConnMaxLifetime(lifetime)
	// the connection is in use again: its pending idle timer must not
	// close it while it is checked out
	dbpo.idleGen++
	if dbpo.idleTimer != nil {
		dbpo.idleTimer.Stop()
		dbpo.idleTimer = nil
	}
	return dbpo
}

// Open returns a pooled connection for connStr, opening and registering a
// new one when the pool has capacity.
func (p *Pool) Open(connStr string, options ...string) *DBP {
	trc.Trace1("pooling.go: Open() - ENTRY")
	trc.Trace1("connStr=" + maskDSN(connStr))

	var Time time.Duration
	configuredLifetime := false
	count := len(options)
	if count > 0 {
		for i := range count {
			opt := strings.SplitN(options[i], "=", 2)
			if len(opt) == 2 && opt[0] == "SetConnMaxLifetime" {
				n, err := strconv.Atoi(opt[1])
				if err != nil || n <= 0 {
					n = defaultConnMaxLifetime
				}
				Time = time.Duration(n) * time.Second
				configuredLifetime = true
			} else {
				fmt.Println("Not a valid parameter")
			}
		}
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	if configuredLifetime {
		p.maxLifetime = Time
	} else {
		Time = p.maxLifetime
	}
	timeout := p.maxLifetime
	p.mu.Unlock()
	deadline := time.Now().Add(timeout)
	for {
		p.mu.Lock()
		// re-check on every iteration: Release() may have nil'ed the
		// pool maps while we were waiting for a free slot
		if p.closed {
			p.mu.Unlock()
			return nil
		}
		if dbpo := p.takeAvailableLocked(connStr, Time); dbpo != nil {
			p.mu.Unlock()
			return dbpo
		}
		if p.size() < p.poolSize {
			db, err := sql.Open("go_ibm_db", connStr)
			if err != nil {
				p.mu.Unlock()
				return nil
			}
			dbi := &DBP{
				DB:   db,
				con:  connStr,
				n:    Time,
				pool: p,
			}
			p.usedPool[connStr] = append(p.usedPool[connStr], dbi)
			dbi.SetConnMaxLifetime(Time)
			p.mu.Unlock()
			return dbi
		}
		p.mu.Unlock()

		if !time.Now().Before(deadline) {
			fmt.Println("Connection timeout")
			trc.Trace1("Connection timeout")
			trc.Trace1("pooling.go: Open() - EXIT")
			return nil
		}
		time.Sleep(3 * time.Second)
	}
}

// Init eagerly opens numConn connections into the pool.
func (p *Pool) Init(numConn int, connStr string) bool {
	trc.Trace1("pooling.go: Init() - ENTRY")
	trc.Trace1(fmt.Sprintf("numConn=%d, connStr=%s", numConn, maskDSN(connStr)))

	var Time time.Duration

	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return false
	}
	Time = p.maxLifetime
	p.mu.Unlock()

	for range numConn {
		db, err := sql.Open("go_ibm_db", connStr)
		if err != nil {
			trc.Trace1("pooling.go: Init() - return false")
			return false
		}
		dbi := &DBP{
			DB:   db,
			con:  connStr,
			n:    Time,
			pool: p,
		}
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			_ = db.Close()
			trc.Trace1("pooling.go: Init() - return false")
			return false
		}
		p.availablePool[connStr] = append(p.availablePool[connStr], dbi)
		dbi.SetConnMaxLifetime(Time)
		p.mu.Unlock()
	}
	trc.Trace1("pooling.go: Init() - EXIT")
	return true
}

// Close returns the connection to its pool, making it available for the
// next Open.
func (d *DBP) Close() {
	trc.Trace1("pooling.go: Close() - ENTRY")

	p := d.pool
	if p == nil {
		_ = d.DB.Close()
		return
	}
	found := false
	var pos int
	p.mu.Lock()
	if valc, okc := p.usedPool[d.con]; okc {
		for i, v := range valc {
			if v == d {
				pos = i
				found = true
				break
			}
		}
		if found {
			if len(valc) > 1 {
				dbpc := valc[pos]
				copy(valc[pos:], valc[pos+1:])
				valc[len(valc)-1] = nil
				valc = valc[:len(valc)-1]
				p.usedPool[d.con] = valc
				p.availablePool[d.con] = append(p.availablePool[d.con], dbpc)
			} else {
				dbpc := valc[0]
				p.availablePool[d.con] = append(p.availablePool[d.con], dbpc)
				delete(p.usedPool, d.con)
			}
			// reset the idle timer on every return: a stale timer from
			// an earlier return must not close the reused connection
			// before its new lifetime has elapsed
			d.idleGen++
			gen := d.idleGen
			if d.idleTimer != nil {
				d.idleTimer.Stop()
			}
			d.idleTimer = time.AfterFunc(d.n, func() { d.expire(gen) })
		} else {
			_ = d.DB.Close()
		}
	} else {
		_ = d.DB.Close()
	}
	p.mu.Unlock()
	trc.Trace1("pooling.go: Close() - EXIT")
}

// expire is the idle-timer callback: it removes the connection from the
// pool and closes it, but only if the connection has been sitting idle in
// the available pool since generation gen (reissue or re-return bumps the
// generation and invalidates this callback).
func (d *DBP) expire(gen uint64) {
	trc.Trace1("pooling.go: expire() - ENTRY")

	p := d.pool
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || gen != d.idleGen {
		return
	}
	valt, okt := p.availablePool[d.con]
	if !okt {
		return
	}
	for i, v := range valt {
		if v != d {
			continue
		}
		copy(valt[i:], valt[i+1:])
		valt[len(valt)-1] = nil
		valt = valt[:len(valt)-1]
		if len(valt) == 0 {
			delete(p.availablePool, d.con)
		} else {
			p.availablePool[d.con] = valt
		}
		d.idleTimer = nil
		_ = d.DB.Close()
		trc.Trace1("pooling.go: expire() - EXIT")
		return
	}
}

// Timeout closes the pooled connection once its lifetime expires.
//
// Deprecated: the pool now manages idle expiry internally; this manual
// trigger is kept for backward compatibility.
func (d *DBP) Timeout() {
	trc.Trace1("pooling.go: Timeout() - ENTRY")

	p := d.pool
	<-time.After(d.n)
	if p == nil {
		return
	}
	found := false
	var pos int
	p.mu.Lock()
	if valt, okt := p.availablePool[d.con]; okt {
		for i, v := range valt {
			if v == d {
				pos = i
				found = true
				break
			}
		}
		if found {
			dbpt := valt[pos]
			copy(valt[pos:], valt[pos+1:])
			valt[len(valt)-1] = nil
			valt = valt[:len(valt)-1]
			if len(valt) == 0 {
				delete(p.availablePool, d.con)
			} else {
				p.availablePool[d.con] = valt
			}
			_ = dbpt.DB.Close()
		}
	}
	p.mu.Unlock()
	trc.Trace1("pooling.go: Timeout() - EXIT")
}

// Release closes the pool and all connections it owns.
func (p *Pool) Release() {
	trc.Trace1("pooling.go: Release() - ENTRY")

	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	if p.availablePool != nil {
		for _, vala := range p.availablePool {
			for _, dbpr := range vala {
				_ = dbpr.DB.Close()
			}
		}
		p.availablePool = nil
	}
	if p.usedPool != nil {
		for _, valu := range p.usedPool {
			for _, dbpr := range valu {
				_ = dbpr.DB.Close()
			}
		}
		p.usedPool = nil
	}
	p.mu.Unlock()
	trc.Trace1("pooling.go: Release() - EXIT")
}

// Display prints the pool maps and size for debugging.
//
// Deprecated: debug helper kept for backward compatibility only.
func (p *Pool) Display() {
	fmt.Println(p.availablePool)
	fmt.Println(p.usedPool)
	fmt.Println(p.poolSize)
}

// SetConnMaxLifetime sets the pooled connection lifetime in seconds.
func (p *Pool) SetConnMaxLifetime(num int) {
	trc.Trace1("pooling.go: SetConnMaxLifetime()")
	trc.Trace1(fmt.Sprintf("connMaxLifetime=%d", num))

	if num <= 0 {
		num = defaultConnMaxLifetime
	}
	p.mu.Lock()
	p.maxLifetime = time.Duration(num) * time.Second
	p.mu.Unlock()
}
