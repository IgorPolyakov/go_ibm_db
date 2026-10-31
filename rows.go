// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package go_ibm_db

import (
	"context"
	"database/sql/driver"
	"fmt"
	"io"
	"reflect"
	"unsafe"

	"github.com/ibmdb/go_ibm_db/api"
	trc "github.com/ibmdb/go_ibm_db/log2"
)

// Rows iterates a result set, serving rows one by one or in blocks in array fetch mode.
type Rows struct {
	os           *ODBCStmt
	ctx          context.Context
	batchSize    int
	rowsInBatch  int
	currentIndex int
}

func newRows(ctx context.Context, os *ODBCStmt) *Rows { //nolint:contextcheck // nil ctx is an intentional default for the args-free Conn.Query path
	if ctx == nil {
		ctx = context.Background()
	}
	batchSize := os.fetchSize
	if batchSize < 1 {
		batchSize = 1
	}
	return &Rows{
		os:        os,
		ctx:       ctx,
		batchSize: batchSize,
	}
}

// Columns returns the column names of the result set.
func (r *Rows) Columns() []string {
	trc.Trace1("rows.go: Columns()")

	names := make([]string, len(r.os.Cols))
	for i := 0; i < len(names); i++ {
		names[i] = r.os.Cols[i].Name()
	}
	return names
}

// ColumnTypePrecisionScale returns the precision and scale of a column when known.
func (r *Rows) ColumnTypePrecisionScale(index int) (precision, scale int64, ok bool) {
	// returns the precision and scale of the column when known
	trc.Trace1("rows.go: ColumnTypePrecisionScale() - ENTRY")
	trc.Trace1(fmt.Sprintf("index=%d", index))

	ok = false
	var namelen api.SQLSMALLINT
	namebuf := make([]byte, api.MAX_FIELD_SIZE)
	ret := api.SQLColAttribute(r.os.h, api.SQLUSMALLINT(index+1), api.SQL_DESC_TYPE_NAME, api.SQLPOINTER(unsafe.Pointer(&namebuf[0])), api.MAX_FIELD_SIZE, (*api.SQLSMALLINT)(&namelen), api.SQLPOINTER(unsafe.Pointer(nil)))

	if IsError(ret) {
		fmt.Println(ret)
		trc.Trace1(fmt.Sprintf("Error: %d", ret))
		return 0, 0, false
	}
	dbtype := string(namebuf[:namelen])
	ret = api.SQLColAttribute(r.os.h, api.SQLUSMALLINT(index+1), api.SQL_DESC_PRECISION, api.SQLPOINTER(unsafe.Pointer(nil)), 0, (*api.SQLSMALLINT)(nil), api.SQLPOINTER(unsafe.Pointer(&precision)))
	if IsError(ret) {
		fmt.Println(ret)
		trc.Trace1(fmt.Sprintf("Error: %d", ret))
		return 0, 0, false
	}
	ret = api.SQLColAttribute(r.os.h, api.SQLUSMALLINT(index+1), api.SQL_DESC_SCALE, api.SQLPOINTER(unsafe.Pointer(nil)), 0, (*api.SQLSMALLINT)(nil), api.SQLPOINTER(unsafe.Pointer(&scale)))
	if IsError(ret) {
		fmt.Println(ret)
		trc.Trace1(fmt.Sprintf("Error: %d", ret))
		return 0, 0, false
	}
	switch dbtype {
	case "DECIMAL":
		ok = true
	case "NUMERIC":
		ok = true
	case "TIMESTAMP":
		ok = true
	}

	trc.Trace1(fmt.Sprintf("precision=%d, scale=%d", precision, scale))
	trc.Trace1("rows.go: ColumnTypePrecisionScale() - EXIT")
	return precision, scale, ok
}

// ColumnTypeLength returns the length of a column when known.
func (r *Rows) ColumnTypeLength(index int) (length int64, ok bool) {
	trc.Trace1("rows.go: ColumnTypeLength() - ENTRY")

	ret := api.SQLColAttribute(r.os.h, api.SQLUSMALLINT(index+1), api.SQL_DESC_LENGTH, api.SQLPOINTER(unsafe.Pointer(nil)), 0, (*api.SQLSMALLINT)(nil), api.SQLPOINTER(unsafe.Pointer(&length)))
	if IsError(ret) {
		fmt.Println(ret)
		return 0, false
	}
	trc.Trace1("rows.go: ColumnTypeLength() - EXIT")
	return length, true
}

// ColumnTypeNullable returns whether a column is nullable when known.
func (r *Rows) ColumnTypeNullable(index int) (nullable, ok bool) {
	// returns whether the column is nullable when known
	trc.Trace1("rows.go: ColumnTypeNullable() - ENTRY")

	var null int64
	ret := api.SQLColAttribute(r.os.h, api.SQLUSMALLINT(index+1), api.SQL_DESC_NULLABLE, api.SQLPOINTER(unsafe.Pointer(nil)), 0, (*api.SQLSMALLINT)(nil), api.SQLPOINTER(unsafe.Pointer(&null)))
	if IsError(ret) {
		fmt.Println(ret)
		return false, false
	}
	if null == api.SQL_NULLABLE {
		return true, true
	}
	trc.Trace1("rows.go: ColumnTypeNullable() - EXIT")
	return false, true
}

// ColumnTypeScanType returns the scan type of a column.
func (r *Rows) ColumnTypeScanType(index int) reflect.Type {
	// returns the scan type used to scan the column
	// the data to the golang variable.
	trc.Trace1("rows.go: ColumnTypeScanType()")
	trc.Trace1(fmt.Sprintf("index=%d", index))

	a := r.os.Cols[index].TypeScan()
	return a
}

// ColumnTypeDatabaseTypeName returns the database type name (VARCHAR, DECIMAL, ...) of a column.
func (r *Rows) ColumnTypeDatabaseTypeName(index int) string {
	// returns the database type name (VARCHAR, DECIMAL, ...) of the column
	// namebuf can be of uint8 or byte
	trc.Trace1("rows.go: ColumnTypeDatabaseTypeName() - ENTRY")

	var namelen api.SQLSMALLINT
	namebuf := make([]byte, api.MAX_FIELD_SIZE)
	ret := api.SQLColAttribute(r.os.h, api.SQLUSMALLINT(index+1), api.SQL_DESC_TYPE_NAME, api.SQLPOINTER(unsafe.Pointer(&namebuf[0])), api.MAX_FIELD_SIZE, (*api.SQLSMALLINT)(&namelen), api.SQLPOINTER(unsafe.Pointer(nil)))

	if IsError(ret) {
		fmt.Println(ret)
		return ""
	}
	dbtype := string(namebuf[:namelen])
	trc.Trace1("rows.go: ColumnTypeDatabaseTypeName() - EXIT")
	return dbtype
}

// Next fetches the next row into dest.
func (r *Rows) Next(dest []driver.Value) error {
	trc.Trace1("rows.go: Next() - ENTRY")

	// If the query context is canceled while SQLFetch is blocked on the
	// server, ask the server to abort so the call returns promptly.
	stopWatch := watchContext(r.ctx, r.os)
	defer stopWatch()

	// Bulk fetch mode: serve rows from the fetched block
	if r.batchSize > 1 {
		for {
			if r.currentIndex >= r.rowsInBatch {
				r.currentIndex = 0
				r.os.rowsFetched = 0
				ret := api.SQLFetch(r.os.h)
				if ret == api.SQL_NO_DATA {
					return io.EOF
				}
				if IsError(ret) {
					if cerr := r.ctx.Err(); cerr != nil {
						return cerr
					}
					return NewError("SQLFetch", r.os.h)
				}
				r.rowsInBatch = int(r.os.rowsFetched)
				if r.rowsInBatch == 0 {
					return io.EOF
				}
			}
			row := r.currentIndex
			r.currentIndex++
			// NOROW is a gap in the row set and can be skipped, but an
			// error status must surface to the caller instead of silently
			// dropping the row and returning an incomplete result
			switch r.os.rowStatus[row] {
			case api.SQL_ROW_SUCCESS, api.SQL_ROW_SUCCESS_WITH_INFO:
				// fall through to read the values below
			case api.SQL_ROW_NOROW:
				continue
			default:
				return NewError("SQLFetch", r.os.h)
			}
			for i := range dest {
				v, err := r.os.Cols[i].ValueAtRow(r.os.h, i, row)
				if err != nil {
					return err
				}
				dest[i] = v
			}
			trc.Trace1("rows.go: Next() - EXIT")
			return nil
		}
	}

	// Single row fetch (default)
	ret := api.SQLFetch(r.os.h)
	if ret == api.SQL_NO_DATA {
		return io.EOF
	}
	if IsError(ret) {
		if cerr := r.ctx.Err(); cerr != nil {
			return cerr
		}
		return NewError("SQLFetch", r.os.h)
	}
	for i := range dest {
		v, err := r.os.Cols[i].Value(r.os.h, i)
		if err != nil {
			return err
		}
		dest[i] = v
	}
	trc.Trace1("rows.go: Next() - EXIT")
	return nil
}

// HasNextResultSet reports whether another result set is available.
func (r *Rows) HasNextResultSet() bool {
	trc.Trace1("rows.go: HasVextResultSet()")

	return true
}

// NextResultSet advances to the next result set of a batch.
func (r *Rows) NextResultSet() error {
	trc.Trace1("rows.go: NextResultSet() - ENTRY")

	ret := api.SQLMoreResults(r.os.h)
	if ret == api.SQL_NO_DATA {
		return io.EOF
	}
	if IsError(ret) {
		return NewError("SQLMoreResults", r.os.h)
	}

	err := r.os.BindColumns()
	if err != nil {
		return err
	}
	r.rowsInBatch = 0
	r.currentIndex = 0
	trc.Trace1("rows.go: NextResultSet() - EXIT")
	return nil
}

// Close closes the cursor and releases the statement for reuse by its Stmt.
func (r *Rows) Close() error {
	trc.Trace1("rows.go: Close()")

	return r.os.closeByRows()
}
