// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package go_ibm_db implements the database/sql driver for IBM DB2
// (and Informix via DRDA) on top of the DB2 CLI (ODBC) API.
package go_ibm_db

import (
	"database/sql"
	"fmt"
	"os"

	"github.com/ibmdb/go_ibm_db/api"
	trc "github.com/ibmdb/go_ibm_db/log2"
)

var drv Driver

// Driver is the database/sql driver registered under the name go_ibm_db.
type Driver struct {
	Stats
	h       api.SQLHENV // environment handle
	initErr error
}

func initDriver() error {
	trc.Trace1("driver.go:InitDriver() - ENTRY")

	// Allocate environment handle
	var out api.SQLHANDLE
	in := api.SQLHANDLE(api.SQL_NULL_HANDLE)
	ret := api.SQLAllocHandle(api.SQL_HANDLE_ENV, in, &out)
	if IsError(ret) {
		return NewError("SQLAllocHandle", api.SQLHENV(in))
	}
	drv.h = api.SQLHENV(out)
	drv.updateHandleCount(api.SQL_HANDLE_ENV, 1)

	// will use ODBC v3
	ret = api.SQLSetEnvUIntPtrAttr(drv.h, api.SQL_ATTR_ODBC_VERSION,
		api.SQL_OV_ODBC3, 0)
	if IsError(ret) {
		defer func() { _ = releaseHandle(drv.h) }()
		return NewError("SQLSetEnvAttr ODBC v3", drv.h)
	}

	trc.Trace1("driver.go:InitDriver() - EXIT")
	return nil
}

// Close releases the driver environment handle; database/sql never calls it, the handle lives for the process lifetime.
func (d *Driver) Close() error {
	trc.Trace1("driver.go: Close() - ENTRY")

	// Note: database/sql never calls Driver.Close; the environment handle
	// lives for the process lifetime (see todo.md).
	h := d.h
	d.h = api.SQLHENV(api.SQL_NULL_HENV)

	trc.Trace1("driver.go: Close() - EXIT")
	return releaseHandle(h)
}

func init() {
	trc.GetPath(os.Getenv("GO_IBMDB_TRACE"))

	trc.Trace1("driver.go:init() - ENTRY")

	func() {
		defer func() {
			if err := recover(); err != nil {
				drv.initErr = fmt.Errorf("initialize go_ibm_db: %v", err)
			}
		}()
		drv.initErr = initDriver()
	}()

	sql.Register("go_ibm_db", &drv)

	trc.Trace1("driver.go:init() - EXIT")
}
