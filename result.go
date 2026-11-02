// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package go_ibm_db

import (
	"context"
	"database/sql/driver"
	"fmt"
	"strconv"

	trc "github.com/ibmdb/go_ibm_db/log2"
)

// Result is the driver.Result of an Exec call.
type Result struct {
	rowCount      int64
	lastInsertID  int64
	lastInsertErr error
}

// LastInsertId returns the identity value captured before Exec released its connection.
func (r *Result) LastInsertId() (int64, error) {
	return r.lastInsertID, r.lastInsertErr
}

func (c *Conn) lastInsertID(ctx context.Context) (int64, error) {
	trc.Trace1("result.go: LastInsetId() - ENTRY")

	lastInsertID := int64(0)

	s, err := c.PrepareContext(ctx, c.dbms.lastInsertIDQuery())
	if err != nil {
		return 0, err
	}
	defer func() { _ = s.Close() }()

	stmt, ok := s.(*Stmt)
	if !ok {
		return 0, fmt.Errorf("%w: %T", errUnexpectedStatementType, s)
	}
	rows, err := stmt.query1(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()

	dest := make([]driver.Value, 1)
	err = rows.Next(dest)
	if err != nil {
		return 0, err
	}

	for _, val := range dest {
		switch v := val.(type) {
		case []byte:
			lastInsertID, err = strconv.ParseInt(string(v), 10, 64)
			if err != nil {
				return 0, err
			}
		case int64:
			lastInsertID = v
		case float64:
			lastInsertID = int64(v)
		case string:
			lastInsertID, err = strconv.ParseInt(v, 10, 64)
			if err != nil {
				return 0, err
			}
		case nil:
			// no identity value available (e.g. non-identity insert)
		default:
			return 0, errUnexpectedType
		}
	}
	trc.Trace1("LastInsertid = %d", lastInsertID)
	trc.Trace1("result.go: LastInsetId() - EXIT")

	return lastInsertID, nil
}

// RowsAffected returns the number of rows affected by the statement.
func (r *Result) RowsAffected() (int64, error) {
	return r.rowCount, nil
}
