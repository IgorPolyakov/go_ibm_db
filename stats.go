// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package go_ibm_db

import (
	"fmt"
	"sync"

	"github.com/ibmdb/go_ibm_db/api"
	trc "github.com/ibmdb/go_ibm_db/log2"
)

// Stats counts allocated ODBC handles by type, exposed for diagnostics.
type Stats struct {
	EnvCount  int
	ConnCount int
	StmtCount int
	mu        sync.Mutex
}

// Snapshot returns the current handle counts under the stats lock.
func (s *Stats) Snapshot() (env, conn, stmt int) {
	trc.Trace1("stats.go: Snapshot()")

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.EnvCount, s.ConnCount, s.StmtCount
}

// HandleCounts reports the driver's live ODBC handle counts (environment,
// connection and statement handles) for diagnostics and leak checks.
func HandleCounts() (env, conn, stmt int) {
	return drv.Snapshot()
}

func (s *Stats) updateHandleCount(handleType api.SQLSMALLINT, change int) {
	trc.Trace1("stats.go: updateHandleCount() - ENTRY")
	trc.Trace1("change=%d", change)

	s.mu.Lock()
	defer s.mu.Unlock()
	switch handleType {
	case api.SQL_HANDLE_ENV:
		s.EnvCount += change
	case api.SQL_HANDLE_DBC:
		s.ConnCount += change
	case api.SQL_HANDLE_STMT:
		s.StmtCount += change
	default:
		trc.Trace1("unexpected handle type %d", handleType)
		panic(fmt.Errorf("%w: %d", errUnexpectedHandleType, handleType))
	}

	trc.Trace1("stats.go: updateHandleCount() - EXIT")
}
