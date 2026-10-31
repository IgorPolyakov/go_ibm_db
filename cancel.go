// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package go_ibm_db

import (
	"context"
	"sync"
)

// watchContext arranges for st.cancel() to run as soon as ctx is done, so
// that a blocked ODBC call (SQLExecute, SQLFetch) is aborted instead of
// pinning the goroutine until the server responds. The returned stop
// function must be called when the watched operation has finished so the
// watcher goroutine does not outlive the statement.
func watchContext(ctx context.Context, st *ODBCStmt) (stop func()) {
	done := make(chan struct{})
	var once sync.Once
	go func() {
		select {
		case <-ctx.Done():
			st.cancel()
		case <-done:
		}
	}()
	return func() { once.Do(func() { close(done) }) }
}
