// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package go_ibm_db

import (
	"database/sql/driver"
	"fmt"
	"runtime"
	"time"
	"unsafe"

	"github.com/ibmdb/go_ibm_db/api"
	trc "github.com/ibmdb/go_ibm_db/log2"
)

// SQLExecDirect is used by Conn.Query for queries without parameters;
// everything else goes through prepare/execute.

// ODBCStmt is a prepared ODBC statement: its handle, bound parameters and
// result-set columns, shared between Stmt and Rows.
type ODBCStmt struct {
	h          api.SQLHSTMT
	Parameters []Parameter
	Cols       []Column
	// locking/lifetime
	mu         mutex
	usedByStmt bool
	usedByRows bool
	// cancellation guard: serializes SQLCancel (fired from a context
	// watcher goroutine) with statement handle release
	cancelMu mutex
	freed    bool
	// bulk fetch support
	fetchSize      int
	rowsFetched    *api.SQLULEN
	rowsFetchedPin runtime.Pinner
	rowStatus      []api.SQLUSMALLINT
	rowStatusPin   *runtime.Pinner
	currentRow     int
}

// PrepareODBCStmt allocates a statement handle and prepares the query on
// the connection.
func (c *Conn) PrepareODBCStmt(query string) (*ODBCStmt, error) {
	trc.Trace1("odbcstmt.go: PrepareODBCStmt() - ENTRY")

	var out api.SQLHANDLE
	ret := api.SQLAllocHandle(api.SQL_HANDLE_STMT, api.SQLHANDLE(c.h), &out)
	if IsError(ret) {
		return nil, NewError("SQLAllocHandle", c.h)
	}
	h := api.SQLHSTMT(out)
	drv.updateHandleCount(api.SQL_HANDLE_STMT, 1)
	b := api.StringToUTF16(query)
	if runtime.GOOS == "zos" {
		// On z/OS, StringToUTF16 does not append null terminator,
		// so we must pass the exact byte length instead of SQL_NTS
		ret = api.SQLPrepare(h,
			(*api.SQLWCHAR)(unsafe.Pointer(&b[0])), api.SQLINTEGER(2*len(b))) //nolint:gosec // bounded query length
	} else {
		ret = api.SQLPrepare(h,
			(*api.SQLWCHAR)(unsafe.Pointer(&b[0])), api.SQL_NTS)
	}
	if IsError(ret) {
		defer func() { _ = releaseHandle(h) }()
		return nil, NewError("SQLPrepare", h)
	}
	ps, err := ExtractParameters(h, query)
	if err != nil {
		defer func() { _ = releaseHandle(h) }()
		return nil, err
	}

	trc.Trace1("odbcstmt.go: PrepareODBCStmt() - EXIT")
	return &ODBCStmt{
		h:          h,
		Parameters: ps,
		usedByStmt: true,
		fetchSize:  c.fetchSize,
	}, nil
}

func (s *ODBCStmt) closeByStmt() error {
	trc.Trace1("odbcstmt.go: closeByStmt() - ENTRY")

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.usedByStmt {
		defer func() { s.usedByStmt = false }()
		if !s.usedByRows {
			return s.releaseHandle()
		}
	}
	trc.Trace1("odbcstmt.go: closeByStmt() - EXIT")
	return nil
}

func (s *ODBCStmt) closeByRows() error {
	trc.Trace1("odbcstmt.go: closeByRows()")

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.usedByRows {
		defer func() { s.usedByRows = false }()
		if s.usedByStmt {
			ret := api.SQLCloseCursor(s.h)
			if IsError(ret) {
				return NewError("SQLCloseCursor", s.h)
			}
			return nil
		}
		return s.releaseHandle()
	}
	return nil
}

func (s *ODBCStmt) releaseHandle() error {
	trc.Trace1("odbcstmt.go: releaseHandle()")

	s.cancelMu.Lock()
	s.freed = true
	s.cancelMu.Unlock()

	h := s.h
	s.h = api.SQLHSTMT(api.SQL_NULL_HSTMT)
	err := releaseHandle(h)
	if s.rowStatusPin != nil {
		s.rowStatusPin.Unpin()
	}
	s.rowsFetchedPin.Unpin()
	return err
}

// cancel aborts an in-flight operation on this statement. It is safe to
// call from another goroutine while SQLExecute or SQLFetch is blocked:
// ODBC explicitly permits SQLCancel from a different thread. If the
// statement handle has already been released, cancel does nothing.
func (s *ODBCStmt) cancel() {
	s.cancelMu.Lock()
	defer s.cancelMu.Unlock()
	if s.freed || s.h == api.SQLHSTMT(api.SQL_NULL_HSTMT) {
		return
	}
	api.SQLCancel(s.h)
}

func (s *ODBCStmt) Exec(args []driver.Value) error {
	trc.Trace1("odbcstmt.go: Exec() - ENTRY")

	ArrayCheck := 0
	ArrayLength := 0
	if len(args) != len(s.Parameters) {
		return fmt.Errorf("%w: %d given, %d expected", ErrWrongArgumentCount, len(args), len(s.Parameters))
	}
	for i, a := range args {
		// this could be done in 2 steps:
		// 1) bind vars right after prepare;
		// 2) set their (vars) values here;
		// but rebinding parameters for every new parameter value
		// should be efficient enough for our purpose.
		err := s.Parameters[i].BindValue(s.h, i, a)
		if err != nil {
			return err
		}
	}

	for _, a := range args {
		if ArrayLength == 0 {
			switch d := a.(type) {
			case []int64:
				ArrayLength = len(d)
				ArrayCheck = 1
			case []string:
				ArrayLength = len(d)
				ArrayCheck = 1
			case []bool:
				ArrayLength = len(d)
				ArrayCheck = 1
			case []float64:
				ArrayLength = len(d)
				ArrayCheck = 1
			case []time.Time:
				ArrayLength = len(d)
				ArrayCheck = 1
			}
		} else {
			switch d := a.(type) {
			case []int64:
				if len(d) != ArrayLength {
					return ErrParameterArrayLength
				}
				ArrayLength = len(d)
				ArrayCheck = 1
			case []string:
				if len(d) != ArrayLength {
					return ErrParameterArrayLength
				}
				ArrayLength = len(d)
				ArrayCheck = 1
			case []bool:
				if len(d) != ArrayLength {
					return ErrParameterArrayLength
				}
				ArrayLength = len(d)
				ArrayCheck = 1
			case []float64:
				if len(d) != ArrayLength {
					return ErrParameterArrayLength
				}
				ArrayLength = len(d)
				ArrayCheck = 1
			case []time.Time:
				if len(d) != ArrayLength {
					return ErrParameterArrayLength
				}
				ArrayLength = len(d)
				ArrayCheck = 1
			}
		}
	}

	paramSetSize := 1
	if ArrayCheck == 1 {
		paramSetSize = ArrayLength
	}
	ret := api.SQLSetStmtUIntPtrAttr(s.h, api.SQL_ATTR_PARAMSET_SIZE,
		uintptr(paramSetSize), api.SQL_IS_INTEGER)
	if IsError(ret) {
		return NewError("SQLSetStmtAttr", s.h)
	}

	ret = api.SQLExecute(s.h)
	if ret == api.SQL_NO_DATA {
		// success but no data to report
		return nil
	}
	if IsError(ret) {
		return NewError("SQLExecute", s.h)
	}
	for _, p := range s.Parameters {
		for _, o := range p.Outs {
			if err := o.ConvertAssign(); err != nil {
				return err
			}
		}
	}
	trc.Trace1("odbcstmt.go: Exec() - EXIT")
	return nil
}

func (s *ODBCStmt) BindColumns() error {
	trc.Trace1("odbcstmt.go: BindColumns() - ENTRY")

	s.currentRow = 0
	if s.rowsFetched != nil {
		*s.rowsFetched = 0
	}

	// count columns
	var n api.SQLSMALLINT
	ret := api.SQLNumResultCols(s.h, &n)
	if IsError(ret) {
		return NewError("SQLNumResultCols", s.h)
	}
	if n < 1 {
		return ErrNoResultSet
	}

	// Set up block cursor if fetchSize > 1
	if s.fetchSize > 1 {
		if s.rowsFetched == nil {
			s.rowsFetched = new(api.SQLULEN)
			s.rowsFetchedPin.Pin(s.rowsFetched)
		}
		ret = api.SQLSetStmtUIntPtrAttr(s.h, api.SQL_ATTR_ROW_ARRAY_SIZE,
			uintptr(s.fetchSize), api.SQL_IS_UINTEGER)
		if IsError(ret) {
			return NewError("SQLSetStmtAttr SQL_ATTR_ROW_ARRAY_SIZE", s.h)
		}
		ret = api.SQLSetStmtAttr(s.h, api.SQL_ATTR_ROWS_FETCHED_PTR,
			api.SQLPOINTER(unsafe.Pointer(s.rowsFetched)), 0)
		if IsError(ret) {
			return NewError("SQLSetStmtAttr SQL_ATTR_ROWS_FETCHED_PTR", s.h)
		}
		rowStatus := make([]api.SQLUSMALLINT, s.fetchSize)
		rowStatusPin := new(runtime.Pinner)
		rowStatusPin.Pin(&rowStatus[0])
		ret = api.SQLSetStmtAttr(s.h, api.SQL_ATTR_ROW_STATUS_PTR,
			api.SQLPOINTER(unsafe.Pointer(&rowStatus[0])), 0)
		if IsError(ret) {
			rowStatusPin.Unpin()
			return NewError("SQLSetStmtAttr SQL_ATTR_ROW_STATUS_PTR", s.h)
		}
		if s.rowStatusPin != nil {
			s.rowStatusPin.Unpin()
		}
		s.rowStatus = rowStatus
		s.rowStatusPin = rowStatusPin
	}

	// fetch column descriptions
	s.Cols = make([]Column, n)
	binding := true
	for i := range s.Cols {
		c, err := NewColumn(s.h, i)
		if err != nil {
			return err
		}
		s.Cols[i] = c
		// Once we found one non-bindable column, we will not bind the rest.
		// http://www.easysoft.com/developer/languages/c/odbc-tutorial-fetching-results.html
		// ... One common restriction is that SQLGetData may only be called on columns after the last bound column. ...
		if !binding {
			continue
		}
		if s.fetchSize > 1 {
			bound, err := s.Cols[i].BindArray(s.h, i, s.fetchSize)
			if err != nil {
				return err
			}
			if !bound {
				// Fall back to single-row fetch if a column can't be array-bound
				binding = false
				s.fetchSize = 1
				ret = api.SQLSetStmtUIntPtrAttr(s.h, api.SQL_ATTR_ROW_ARRAY_SIZE,
					1, api.SQL_IS_UINTEGER)
				if IsError(ret) {
					return NewError("SQLSetStmtAttr SQL_ATTR_ROW_ARRAY_SIZE reset", s.h)
				}
			}
		} else {
			bound, err := s.Cols[i].Bind(s.h, i)
			if err != nil {
				return err
			}
			if !bound {
				binding = false
			}
		}
	}
	trc.Trace1("odbcstmt.go: BindColumns() - EXIT")
	return nil
}
