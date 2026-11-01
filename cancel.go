// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package go_ibm_db

import (
	"context"
	"sync"
)

// watchContext arranges for st.cancel() to run as soon as ctx is done, so
// that a blocked ODBC call (SQLExecute, SQLFetch, SQLMoreResults) is
// aborted instead of pinning the goroutine until the server responds. The
// returned stop function must be called when the watched operation has
// finished. stop WAITS for the watcher goroutine to exit: if the context
// fires concurrently with the operation finishing, the watcher may already
// be inside SQLCancel, and returning before it completes would let that
// cancel bleed into the next execution on the same statement.
func watchContext(ctx context.Context, st *ODBCStmt) (stop func()) {
	done := make(chan struct{})
	exited := make(chan struct{})
	go func() {
		defer close(exited)
		select {
		case <-ctx.Done():
			st.cancel()
		case <-done:
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			close(done)
			<-exited
		})
	}
}
