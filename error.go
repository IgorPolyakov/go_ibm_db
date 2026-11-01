// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package go_ibm_db

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"unsafe"

	"github.com/ibmdb/go_ibm_db/api"
	trc "github.com/ibmdb/go_ibm_db/log2"
)

// Sentinel errors matchable with errors.Is. The wrapped error carries the
// dynamic context (indexes, types, sizes) via fmt.Errorf's %w.
var (
	// ErrStmtClosed is returned when a statement is used after Close.
	ErrStmtClosed = errors.New("Stmt is closed")
	// ErrStmtAlreadyClosed is returned by Close on an already closed statement.
	ErrStmtAlreadyClosed = errors.New("Stmt is already closed")
	// ErrNoResultSet is returned when an executed query produced no result set.
	ErrNoResultSet = errors.New("Query executed successfully but did not create a result set")
	// ErrWrongArgumentCount is returned when the number of query arguments
	// does not match the number of parameter markers.
	ErrWrongArgumentCount = errors.New("wrong number of arguments")
	// ErrAlreadyInTransaction is returned by Begin on a connection that is
	// already inside a transaction.
	ErrAlreadyInTransaction = errors.New("already in a transaction")
	// ErrNotInTransaction is returned by Commit/Rollback outside a transaction.
	ErrNotInTransaction = errors.New("not in a transaction")
	// ErrParameterArrayLength is returned when array parameter values have
	// differing lengths.
	ErrParameterArrayLength = errors.New("parameter's array value length should be same")
	// ErrEmptyArrayParameter is returned when an array parameter is empty;
	// at least one element is required to derive the binding type.
	ErrEmptyArrayParameter = errors.New("empty array parameter")
)

// Internal sentinel errors used as %w wrap roots.
var (
	errDatabaseNameEmpty        = errors.New("database name cannot be empty")
	errInvalidParameter         = errors.New("not a valid parameter")
	errUnexpectedType           = errors.New("unexpected type")
	errColumnNameBuffer         = errors.New("failed to allocate column name buffer")
	errUnsupportedColumnType    = errors.New("unsupported column type")
	errUnsupportedColumnCType   = errors.New("unsupported column ctype")
	errUnknownColumnWidth       = errors.New("do not know how wide column of ctype")
	errWrongColumnLength        = errors.New("wrong column length returned")
	errRowOutOfRange            = errors.New("row out of range")
	errUnexpectedHandleType     = errors.New("unexpected handle type")
	errInvalidHandle            = errors.New("SQLFreeHandle returns SQL_INVALID_HANDLE")
	errParameterTooLarge        = errors.New("parameter exceeds SQLULEN range")
	errUnsupportedBindParamType = errors.New("unsupported bind param type")
	errUnsupportedParamType     = errors.New("unsupported param type at sql.out")
	errUnexpectedStatementType  = errors.New("unexpected statement type")
	errInOutBufferSize          = errors.New("INOUT string size is greater than the allocated OUT buffer size")
	errUnsupportedOutDestType   = errors.New("unsupported sql.Out.Dest type")
	errUnsupportedOutCType      = errors.New("unsupported ctype for OUT parameter")
	errNilOutParam              = errors.New("sql.Out is nil at OUT param index")
	errNilDest                  = errors.New("variable Dest is nil at OUT param index")
	errDestNotPointer           = errors.New("variable Dest is not a pointer")
	errDestinationPointerNil    = errors.New("destination pointer is nil")
	errDestinationNotPointer    = errors.New("destination not a pointer")
	errUnsupportedScan          = errors.New("unsupported Scan, storing driver.Value")
)

// IsError reports whether an ODBC return code is a failure (anything but SQL_SUCCESS or SQL_SUCCESS_WITH_INFO).
func IsError(ret api.SQLRETURN) bool {
	trc.Trace1("error.go: IsError() - ENTRY")
	switch ret {
	case api.SQL_SUCCESS:
		trc.Trace1("api.SQL_SUCCESS")
	case api.SQL_SUCCESS_WITH_INFO:
		trc.Trace1("api.SQL_SUCCESS_WITH_INFO")
	}
	trc.Trace1("error.go: IsError() - EXIT")
	return ret != api.SQL_SUCCESS && ret != api.SQL_SUCCESS_WITH_INFO
}

// DiagRecord is one ODBC diagnostic record (SQLSTATE, native error and message).
type DiagRecord struct {
	State       string
	NativeError int
	Message     string
}

func (r *DiagRecord) String() string {
	return fmt.Sprintf("{%s} %s", r.State, r.Message)
}

// Error is an ODBC diagnostic error carrying the failing API name and its diagnostic records.
type Error struct {
	APIName string
	Diag    []DiagRecord
}

func (e *Error) Error() string {
	trc.Trace1("error.go: Error() - ENTRY")
	ss := make([]string, len(e.Diag))
	for i, r := range e.Diag {
		ss[i] = r.String()
	}
	trc.Trace1(fmt.Sprintf("%s : %s", e.APIName, ss))
	trc.Trace1("error.go: Error() - EXIT")
	return e.APIName + ": " + strings.Join(ss, "\n")
}

// NewError retrieves the diagnostics of handle and wraps them into an *Error naming the failing API call.
func NewError(apiName string, handle interface{}) error {
	trc.Trace1("error.go: NewError() - ENTRY")
	trc.Trace1("apiName=" + apiName)

	var ret api.SQLRETURN
	h, ht := ToHandleAndType(handle)
	err := &Error{APIName: apiName}
	var ne api.SQLINTEGER
	state := make([]uint16, 6)
	msg := make([]uint16, api.SQL_MAX_MESSAGE_LENGTH)
	for i := 1; ; i++ {
		// odbc api on zos doesn't handle null terminated strings, the exact size is passed
		if runtime.GOOS == "zos" {
			ret = api.SQLGetDiagRec(ht, h, api.SQLSMALLINT(i),
				(*api.SQLWCHAR)(unsafe.Pointer(&state[0])), &ne,
				(*api.SQLWCHAR)(unsafe.Pointer(&msg[0])),
				api.SQLSMALLINT(2*len(msg)), nil) //nolint:gosec // msg is a small fixed-size diag buffer
		} else {
			ret = api.SQLGetDiagRec(ht, h, api.SQLSMALLINT(i),
				(*api.SQLWCHAR)(unsafe.Pointer(&state[0])), &ne,
				(*api.SQLWCHAR)(unsafe.Pointer(&msg[0])),
				api.SQLSMALLINT(len(msg)), nil) //nolint:gosec // msg is a small fixed-size diag buffer
		}
		if ret == api.SQL_NO_DATA {
			break
		}
		if IsError(ret) {
			trc.Trace1(fmt.Sprintf("SQLGetDiagRec failed: ret=%d", ret))
			if len(err.Diag) == 0 {
				err.Diag = append(err.Diag, DiagRecord{Message: fmt.Sprintf("SQLGetDiagRec failed: ret=%d", ret)})
			}
			break
		}
		r := DiagRecord{
			State:       api.UTF16ToString(state),
			NativeError: int(ne),
			Message:     api.UTF16ToString(msg),
		}
		if strings.Contains(r.Message, "CLI0106E") ||
			strings.Contains(r.Message, "CLI0107E") ||
			strings.Contains(r.Message, "CLI0108E") {
			return driver.ErrBadConn
		}
		err.Diag = append(err.Diag, r)
	}
	trc.Trace1(fmt.Sprintf("Error: %s", err))
	trc.Trace1("error.go: NewError() - EXIT")
	return err
}
