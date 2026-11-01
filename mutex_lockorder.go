//go:build lockorder

// This file is compiled only with -tags lockorder: every mutex then
// records its acquisition order per goroutine, and a reversed order
// (B after A somewhere, A after B elsewhere) is reported with both
// stacks. The Go runtime only detects total deadlocks; lock-order
// cycles that do not collide today stay invisible to -race.
package go_ibm_db

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

type mutex struct {
	mu sync.Mutex
	id uintptr // guarded by orderMu (assigned on first use)
}

var (
	orderMu    sync.Mutex
	orderEdges = map[[2]uintptr][]string{} // {from,to} -> example stacks
	nextMuID   uintptr                     // guarded by orderMu
	heldMu     = map[uint64][]uintptr{}    // goroutine id -> held mutex ids
	nameOf     = map[uintptr]string{}      // mutex id -> first acquisition site
)

func goroutineID() uint64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	line := strings.TrimPrefix(string(buf[:n]), "goroutine ")
	if i := strings.IndexByte(line, ' '); i >= 0 {
		line = line[:i]
	}
	id, _ := strconv.ParseUint(line, 10, 64)
	return id
}

func stackTop(skip int) string {
	pc := make([]uintptr, 8)
	n := runtime.Callers(skip+2, pc)
	frames := runtime.CallersFrames(pc[:n])
	for {
		frame, more := frames.Next()
		if strings.Contains(frame.Function, "go_ibm_db.") && !strings.Contains(frame.Function, "mutex.") {
			fn := strings.TrimPrefix(frame.Function, "github.com/ibmdb/go_ibm_db.")
			return fmt.Sprintf("%s:%d", fn, frame.Line)
		}
		if !more {
			return "?"
		}
	}
}

func (m *mutex) Lock() {
	gid := goroutineID()
	here := stackTop(2)

	orderMu.Lock()
	// assign the id under orderMu: two goroutines can first-lock the
	// same mutex concurrently, and an unguarded m.id write is a data
	// race of exactly the kind this build mode exists to detect
	if m.id == 0 {
		nextMuID++
		m.id = nextMuID
		nameOf[m.id] = here
	}
	for _, from := range heldMu[gid] {
		if from == m.id {
			continue
		}
		edge := [2]uintptr{from, m.id}
		if _, seen := orderEdges[edge]; !seen {
			orderEdges[edge] = []string{here}
		}
		reverse := [2]uintptr{m.id, from}
		if where, rseen := orderEdges[reverse]; rseen {
			fmt.Fprintf(os.Stderr, "LOCKORDER: inconsistent order %s -> %s (%s) vs %s -> %s (%s)\n",
				nameOf[from], nameOf[m.id], here, nameOf[m.id], nameOf[from], where[0])
			os.Stderr.Sync()
		}
	}
	heldMu[gid] = append(heldMu[gid], m.id)
	orderMu.Unlock()

	m.mu.Lock()
}

func (m *mutex) Unlock() {
	gid := goroutineID()

	orderMu.Lock()
	held := heldMu[gid]
	for i := len(held) - 1; i >= 0; i-- {
		if held[i] == m.id {
			heldMu[gid] = append(held[:i], held[i+1:]...)
			break
		}
	}
	orderMu.Unlock()

	m.mu.Unlock()
}
