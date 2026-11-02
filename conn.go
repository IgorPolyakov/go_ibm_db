// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package go_ibm_db

import (
	"bytes"
	"context"
	"database/sql/driver"
	"runtime"
	"strconv"
	"strings"
	"unsafe"

	"github.com/ibmdb/go_ibm_db/api"
	trc "github.com/ibmdb/go_ibm_db/log2"
)

// Conn is a single DB2 CLI connection implementing driver.Conn.
type Conn struct {
	h         api.SQLHDBC
	tx        *Tx
	fetchSize int
	dbms      dbmsDialect
}

// parseDSN extracts FETCHSIZE/ROWARRAYSIZE from the connection string and
// returns a cleaned DSN (without those keys) plus the parsed fetch size.
func parseDSN(dsn string) (string, int) {
	fetchSize := 1
	var cleanParts []string
	parts := strings.Split(dsn, ";")
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			continue
		}
		kv := strings.SplitN(trimmed, "=", 2)
		if len(kv) == 2 {
			k := strings.TrimSpace(kv[0])
			v := strings.TrimSpace(kv[1])
			if strings.EqualFold(k, "FETCHSIZE") || strings.EqualFold(k, "ROWARRAYSIZE") {
				if size, err := strconv.Atoi(v); err == nil && size > 0 {
					fetchSize = size
				}
				continue
			}
		}
		cleanParts = append(cleanParts, trimmed)
	}
	cleanDSN := strings.Join(cleanParts, ";")
	if len(cleanParts) > 0 {
		cleanDSN += ";"
	}
	return cleanDSN, fetchSize
}

// maskDSN redacts the password part of a connection string so that
// tracing never logs credentials. The key comparison tolerates spaces
// around the key ("PWD =secret", " PWD=x").
func maskDSN(dsn string) string {
	parts := strings.Split(dsn, ";")
	for i, p := range parts {
		kv := strings.SplitN(p, "=", 2)
		if len(kv) == 2 && strings.EqualFold(strings.TrimSpace(kv[0]), "PWD") {
			parts[i] = kv[0] + "=***"
		}
	}
	return strings.Join(parts, ";")
}

// Open connects using a DSN string (UID, PWD, HOSTNAME, DATABASE, ...).
func (d *Driver) Open(dsn string) (driver.Conn, error) {
	trc.Trace1("conn.go: Open() - ENTRY")
	if d.initErr != nil {
		return nil, d.initErr
	}

	cleanDSN, fetchSize := parseDSN(dsn)

	var out api.SQLHANDLE
	ret := api.SQLAllocHandle(api.SQL_HANDLE_DBC, api.SQLHANDLE(d.h), &out)
	if IsError(ret) {
		return nil, NewError("SQLAllocHandle", d.h)
	}
	h := api.SQLHDBC(out)
	drv.updateHandleCount(api.SQL_HANDLE_DBC, 1)

	b := api.StringToUTF16(cleanDSN)
	if runtime.GOOS == "zos" {
		ret = api.SQLDriverConnect(h, 0,
			(*api.SQLWCHAR)(unsafe.Pointer(&b[0])), api.SQLSMALLINT(2*len(b)), //nolint:gosec // bounded length
			nil, 0, nil, api.SQL_DRIVER_NOPROMPT)
	} else {
		ret = api.SQLDriverConnect(h, 0,
			(*api.SQLWCHAR)(unsafe.Pointer(&b[0])), api.SQLSMALLINT(len(b)), //nolint:gosec // bounded length
			nil, 0, nil, api.SQL_DRIVER_NOPROMPT)
	}
	if IsError(ret) {
		defer func() { _ = releaseHandle(h) }()
		return nil, NewError("SQLDriverConnect", h)
	}
	c := &Conn{h: h, fetchSize: fetchSize}
	name, err := c.GetInfo(api.SQL_DBMS_NAME)
	if err != nil {
		_ = c.Close()
		return nil, err
	}
	c.dbms = dbmsForName(name)
	trc.Trace1("conn.go: Open() - EXIT")
	return c, nil
}

// GetInfo reports a string-valued piece of DBMS/driver information via
// SQLGetInfo, for infoType values such as api.SQL_DBMS_NAME, SQL_DBMS_VER,
// SQL_DRIVER_NAME, SQL_DRIVER_VER, SQL_ODBC_VER, SQL_SERVER_NAME,
// SQL_DATABASE_NAME or SQL_USER_NAME. Callers can reach this through
// database/sql's Conn.Raw, since *Conn implements driver.Conn:
//
//	err := db.Conn(ctx).Raw(func(dc interface{}) error {
//	    name, err := dc.(*go_ibm_db.Conn).GetInfo(api.SQL_DBMS_NAME)
//	    ...
//	})
//
// GetInfo reports a string-valued piece of DBMS/driver information via SQLGetInfo.
func (c *Conn) GetInfo(infoType uint16) (string, error) {
	trc.Trace1("conn.go: GetInfo() - ENTRY")

	buf := make([]uint16, 128)
	var outLen api.SQLSMALLINT
	// BufferLength is in BYTES for the wide API on every platform
	// (z/OS included); passing UTF-16 units instead happens to be
	// tolerated by libdb2 on unix but yields empty values from db2cli64.
	ret := api.SQLGetInfo(c.h, api.SQLUSMALLINT(infoType),
		api.SQLPOINTER(unsafe.Pointer(&buf[0])), api.SQLSMALLINT(2*len(buf)), &outLen) //nolint:gosec // bounded length
	if IsError(ret) {
		return "", NewError("SQLGetInfo", c.h)
	}

	// SQLGetInfo reports the value length in outLen (in BYTES for the
	// wide API, without the terminator). Some CLI builds — notably
	// db2cli64 on Windows — do not NUL-terminate the buffer, so decoding
	// to the first NUL yields an empty string there; honor outLen first
	// and fall back to NUL-termination when it looks unusable.
	name := ""
	units := int(outLen) / 2
	if units > 0 && units <= len(buf) {
		name = api.UTF16ToString(buf[:units])
	}
	if name == "" {
		name = api.UTF16ToString(buf)
	}
	if !isPrintableASCII(name) {
		// Some servers (Informix over DRDA) return the info value as
		// single-byte ASCII even through the wide (UTF-16) API. UTF16ToString
		// then pairs adjacent ASCII bytes into garbage runes; reinterpret the
		// raw bytes in that case.
		raw := make([]byte, 0, 2*len(buf))
		for _, u := range buf {
			raw = append(raw, byte(u), byte(u>>8)) //nolint:gosec // deliberate split of uint16 into its two bytes
		}
		if i := bytes.IndexByte(raw, 0); i >= 0 {
			raw = raw[:i]
		}
		if isPrintableASCII(string(raw)) {
			name = string(raw)
		}
	}

	trc.Trace1("conn.go: GetInfo() - EXIT")
	return name, nil
}

// isPrintableASCII reports whether s consists solely of printable ASCII
// characters (an empty string qualifies).
func isPrintableASCII(s string) bool {
	for _, r := range s {
		if r < 0x20 || r > 0x7e {
			return false
		}
	}
	return true
}

// Close disconnects and frees the connection handle.
func (c *Conn) Close() error {
	trc.Trace1("conn.go: Close() - ENTRY")

	ret := api.SQLDisconnect(c.h)
	if IsError(ret) {
		return NewError("SQLDisconnect", c.h)
	}
	h := c.h
	c.h = api.SQLHDBC(api.SQL_NULL_HDBC)
	trc.Trace1("conn.go: Close() - EXIT")
	return releaseHandle(h)
}

// Query executes the statement without preparing when no args are
// provided, and returns driver.ErrSkip otherwise.
func (c *Conn) Query(query string, args []driver.Value) (driver.Rows, error) {
	trc.Trace1("conn.go: Query() - ENTRY")

	return c.query(context.Background(), query, args)
}

// QueryContext implements driver.QueryerContext: like Query, but a
// canceled or timed-out context aborts the blocking SQLExecDirect call
// via SQLCancel.
func (c *Conn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	trc.Trace1("conn.go: QueryContext() - ENTRY")

	if len(args) > 0 {
		// Not implemented for queries with parameters
		return nil, driver.ErrSkip
	}
	return c.query(ctx, query, nil)
}

func (c *Conn) query(ctx context.Context, query string, args []driver.Value) (driver.Rows, error) {

	if len(args) > 0 {
		// Not implemented for queries with parameters
		return nil, driver.ErrSkip
	}
	var out api.SQLHANDLE
	var os *ODBCStmt
	ret := api.SQLAllocHandle(api.SQL_HANDLE_STMT, api.SQLHANDLE(c.h), &out)
	if IsError(ret) {
		return nil, NewError("SQLAllocHandle", c.h)
	}
	h := api.SQLHSTMT(out)
	drv.updateHandleCount(api.SQL_HANDLE_STMT, 1)
	b := api.StringToUTF16(query)
	stmt := &ODBCStmt{
		h:          h,
		usedByRows: true,
		fetchSize:  c.fetchSize,
	}
	// SQLExecDirect can block on the server: cancel it when ctx is done
	stopWatch := watchContext(ctx, stmt)
	defer stopWatch()
	if runtime.GOOS == "zos" {
		// On z/OS, StringToUTF16 does not append null terminator,
		// so we must pass the exact byte length instead of SQL_NTS
		ret = api.SQLExecDirect(h,
			(*api.SQLWCHAR)(unsafe.Pointer(&b[0])), api.SQLINTEGER(2*len(b))) //nolint:gosec // bounded length
	} else {
		ret = api.SQLExecDirect(h,
			(*api.SQLWCHAR)(unsafe.Pointer(&b[0])), api.SQL_NTS)
	}
	if IsError(ret) {
		defer func() { _ = releaseHandle(h) }()
		if cerr := ctx.Err(); cerr != nil {
			return nil, cerr
		}
		return nil, NewError("SQLExecDirectW", h)
	}
	ps, err := ExtractParameters(h)
	if err != nil {
		defer func() { _ = releaseHandle(h) }()
		return nil, err
	}
	stmt.Parameters = ps
	os = stmt
	err = os.BindColumns()
	if err != nil {
		// release the statement handle on this error path too,
		// otherwise the handle leaks
		defer func() { _ = stmt.releaseHandle() }()
		if cerr := ctx.Err(); cerr != nil {
			return nil, cerr
		}
		return nil, err
	}
	trc.Trace1("conn.go: Query() - EXIT")
	return newRows(ctx, os), nil
}
