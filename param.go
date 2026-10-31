// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package go_ibm_db

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"math"
	"runtime"
	"strings"
	"time"
	"unsafe"

	"github.com/ibmdb/go_ibm_db/api"
	trc "github.com/ibmdb/go_ibm_db/log2"
)

// Parameter is a bound statement parameter: its buffers, types and length/indicator.
type Parameter struct {
	SQLType     api.SQLSMALLINT
	Decimal     api.SQLSMALLINT
	Size        api.SQLULEN
	isDescribed bool
	// Following fields store data used later by SQLExecute.
	// The fields keep data alive and away from gc.
	Data             interface{}
	StrLen_or_IndPtr api.SQLLEN //nolint:revive // ODBC field name kept for compatibility
	Outs             []*Out
}

// StoreStrLen_or_IndPtr stores v into StrLen_or_IndPtr field of p
// and returns address of that field.
func (p *Parameter) StoreStrLen_or_IndPtr(v api.SQLLEN) *api.SQLLEN { //nolint:revive // ODBC name kept for compatibility
	trc.Trace1("param.go: StoreStrLen_or_IndPtr()")

	p.StrLen_or_IndPtr = v
	return &p.StrLen_or_IndPtr
}

// sqlulen converts a non-negative length to SQLULEN, returning an error
// instead of truncating on overflow.
func sqlulen(n int) (api.SQLULEN, error) {
	if n < 0 || uint64(n) > math.MaxUint32 {
		return 0, fmt.Errorf("parameter of %d elements exceeds SQLULEN range", n)
	}
	return api.SQLULEN(n), nil
}

func (p *Parameter) BindValue(h api.SQLHSTMT, idx int, v driver.Value) error {
	// Future optimization: reuse memory for previously bound values. If
	// memory is reused, we, probably, do not need to call SQLBindParameter
	// either (tracked in todo.md).
	trc.Trace1("param.go: BindValue() - ENTRY")

	var ctype, sqltype, decimal api.SQLSMALLINT
	var size api.SQLULEN
	var buflen api.SQLLEN
	var plen *api.SQLLEN
	var buf unsafe.Pointer
	var iotype api.SQLSMALLINT = api.SQL_PARAM_INPUT
	switch d := v.(type) {
	case nil:
		if p.SQLType == api.SQL_BLOB || p.SQLType == api.SQL_VARBINARY || p.SQLType == api.SQL_BINARY {
			ctype = api.SQL_C_BINARY
			sqltype = api.SQL_BINARY
		} else {
			ctype = api.SQL_C_WCHAR
			sqltype = api.SQL_WCHAR
		}
		p.Data = nil
		buf = nil
		size = 1
		buflen = 0
		plen = p.StoreStrLen_or_IndPtr(api.SQL_NULL_DATA)
	case string:
		ctype = api.SQL_C_WCHAR
		b := api.StringToUTF16(d)
		p.Data = b
		buf = unsafe.Pointer(&b[0])
		l := len(b)
		if runtime.GOOS != "zos" {
			l -= 1 // remove terminating 0 added by StringToUTF16 in api.go
		}
		if l < 0 || uint64(l) > math.MaxUint32 {
			return fmt.Errorf("string parameter of %d UTF-16 code units exceeds SQLULEN range", l)
		}
		size = api.SQLULEN(l)
		if size < 1 {
			// size cannot be less then 1 even for empty fields
			size = 1
		}
		l *= 2                 // every char takes 2 bytes
		buflen = api.SQLLEN(l) //nolint:gosec // l is the bounded string length computed above
		plen = p.StoreStrLen_or_IndPtr(buflen)
		if p.isDescribed {
			// only so we can handle very long (>4000 chars) parameters
			sqltype = p.SQLType
		} else {
			sqltype = api.SQL_WCHAR
		}
		//  sqltype is SQL_DECIMAL set p.Decimal
		if sqltype == api.SQL_DECIMAL {
			decimal = p.Decimal
			size = p.Size // SQL_DECIMAL this is the maximum decimal precision
		}
	case int64:
		ctype = api.SQL_C_SBIGINT
		p.Data = &d
		buf = unsafe.Pointer(&d)
		sqltype = api.SQL_BIGINT
		size = 8
	case bool:
		var b int
		if d {
			b = 1
		}
		ctype = api.SQL_C_SBIGINT
		p.Data = &b
		buf = unsafe.Pointer(&b)
		sqltype = api.SQL_BIGINT
		size = 1
	case float64:
		ctype = api.SQL_C_DOUBLE
		p.Data = &d
		buf = unsafe.Pointer(&d)
		sqltype = api.SQL_DOUBLE
		size = 8
	case time.Time:
		ctype = api.SQL_C_TYPE_TIMESTAMP
		y, m, day := d.Date()
		b := api.SQL_TIMESTAMP_STRUCT{
			Year:     api.SQLSMALLINT(y),              //nolint:gosec // calendar fields are structurally bounded
			Month:    api.SQLUSMALLINT(m),             //nolint:gosec
			Day:      api.SQLUSMALLINT(day),           //nolint:gosec
			Hour:     api.SQLUSMALLINT(d.Hour()),      //nolint:gosec
			Minute:   api.SQLUSMALLINT(d.Minute()),    //nolint:gosec
			Second:   api.SQLUSMALLINT(d.Second()),    //nolint:gosec
			Fraction: api.SQLUINTEGER(d.Nanosecond()), //nolint:gosec // Nanosecond() is always < 1e9
		}
		p.Data = &b
		buf = unsafe.Pointer(&b)
		sqltype = api.SQL_TYPE_TIMESTAMP
		if p.isDescribed && p.SQLType == api.SQL_TYPE_TIMESTAMP {
			decimal = p.Decimal
		}
		if decimal <= 0 {
			// represented as yyyy-mm-dd hh:mm:ss.fff format in ms sql server
			decimal = 3
		}
		size = 20 + api.SQLULEN(decimal)
	case []byte:
		ctype = api.SQL_C_BINARY
		b := make([]byte, len(d))
		copy(b, d)
		p.Data = b
		if len(d) > 0 {
			buf = unsafe.Pointer(&b[0])
		}
		buflen = api.SQLLEN(len(b)) //nolint:gosec // len(b) is checked via sqlulen below
		plen = p.StoreStrLen_or_IndPtr(buflen)
		s, err := sqlulen(len(b))
		if err != nil {
			return err
		}
		size = s
		sqltype = api.SQL_BINARY
	case sql.Out:
		o, err := newOut(h, &d, idx)
		if err != nil {
			return err
		}
		iotype = o.inputOutputType
		sqltype = o.sqltype
		ctype = o.ctype
		size = o.parameterSize
		decimal = o.decimalDigits
		b := o.data
		if len(b) > 0 {
			buf = unsafe.Pointer(&b[0])
		}
		buflen = o.buflen
		plen = o.plen
		p.Outs = append(p.Outs, o)
	case []int64:
		if len(d) == 0 {
			return fmt.Errorf("empty array parameter")
		}
		ctype = api.SQL_C_SBIGINT
		b := make([]int64, len(d))
		copy(b, d)
		p.Data = b
		buf = unsafe.Pointer(&b[0])
		buflen = api.SQLLEN(len(b)) //nolint:gosec // len(b) is checked via sqlulen below
		plen = p.StoreStrLen_or_IndPtr(buflen)
		s, err := sqlulen(len(b))
		if err != nil {
			return err
		}
		size = s
		sqltype = api.SQL_BIGINT
	case []string:
		if len(d) == 0 {
			return fmt.Errorf("empty array parameter")
		}
		ctype = api.SQL_C_WCHAR
		maxlen := len(d[0]) + 1
		for i := range d {
			if maxlen <= len(d[i]) {
				maxlen = len(d[i]) + 1
			}
		}
		b := []uint16{}
		for i := range d {
			temp := api.StringToUTF16(d[i])
			if len(temp) < maxlen {
				diff := maxlen - len(temp)
				for range diff {
					temp = append(temp, 0)
				}
			}
			b = append(b, temp...)
		}
		l := maxlen
		p.Data = b
		buf = unsafe.Pointer(&b[0])
		if l < 0 || uint64(l) > math.MaxUint32 {
			return fmt.Errorf("string array parameter of %d UTF-16 code units exceeds SQLULEN range", l)
		}
		size = api.SQLULEN(l)
		if size < 1 {
			// size cannot be less then 1 even for empty fields
			size = 1
		}
		l *= 2                 // every char takes 2 bytes
		buflen = api.SQLLEN(l) //nolint:gosec // l is the bounded string length computed above
		plen = nil
		if p.isDescribed {
			// only so we can handle very long (>4000 chars) parameters
			sqltype = p.SQLType
		} else {
			sqltype = api.SQL_WCHAR
		}
	case []bool:
		if len(d) == 0 {
			return fmt.Errorf("empty array parameter")
		}
		b := make([]int64, len(d))
		for i := range d {
			if d[i] {
				b[i] = 1
			}
		}
		p.Data = b
		ctype = api.SQL_C_SBIGINT
		sqltype = api.SQL_BIGINT
		s, err := sqlulen(len(b))
		if err != nil {
			return err
		}
		size = s
		buf = unsafe.Pointer(&b[0])
		buflen = api.SQLLEN(len(b)) //nolint:gosec // len(b) is the bounded array length
		plen = p.StoreStrLen_or_IndPtr(buflen)
	case []float64:
		if len(d) == 0 {
			return fmt.Errorf("empty array parameter")
		}
		ctype = api.SQL_C_DOUBLE
		b := make([]float64, len(d))
		copy(b, d)
		p.Data = b
		buf = unsafe.Pointer(&b[0])
		buflen = api.SQLLEN(len(b)) //nolint:gosec // len(b) is the bounded array length
		plen = p.StoreStrLen_or_IndPtr(buflen)
		size = 8
		sqltype = api.SQL_DOUBLE
	case []time.Time:
		if len(d) == 0 {
			return fmt.Errorf("empty array parameter")
		}
		switch p.SQLType {
		case api.SQL_TYPE_DATE:
			ctype = api.SQL_C_TYPE_DATE
			b := make([]api.SQL_DATE_STRUCT, len(d))
			for i := range d {
				y, m, day := d[i].Date()
				b[i] = api.SQL_DATE_STRUCT{
					Year:  api.SQLSMALLINT(y),    //nolint:gosec // calendar fields are structurally bounded
					Month: api.SQLUSMALLINT(m),   //nolint:gosec
					Day:   api.SQLUSMALLINT(day), //nolint:gosec
				}
			}
			p.Data = b
			buf = unsafe.Pointer(&b[0])
			sqltype = api.SQL_TYPE_DATE
			size = 10
		case api.SQL_TYPE_TIME:
			ctype = api.SQL_C_TYPE_TIME
			b := make([]api.SQL_TIME_STRUCT, len(d))
			for i := range d {
				b[i] = api.SQL_TIME_STRUCT{
					Hour:   api.SQLUSMALLINT(d[i].Hour()),   //nolint:gosec // calendar fields are structurally bounded
					Minute: api.SQLUSMALLINT(d[i].Minute()), //nolint:gosec
					Second: api.SQLUSMALLINT(d[i].Second()), //nolint:gosec
				}
			}
			p.Data = b
			buf = unsafe.Pointer(&b[0])
			sqltype = api.SQL_TYPE_TIME
			size = 8
		default:
			ctype = api.SQL_C_TYPE_TIMESTAMP
			b := make([]api.SQL_TIMESTAMP_STRUCT, len(d))
			for i := range d {
				y, m, day := d[i].Date()
				b[i] = api.SQL_TIMESTAMP_STRUCT{
					Year:     api.SQLSMALLINT(y),                 //nolint:gosec // calendar fields are structurally bounded
					Month:    api.SQLUSMALLINT(m),                //nolint:gosec
					Day:      api.SQLUSMALLINT(day),              //nolint:gosec
					Hour:     api.SQLUSMALLINT(d[i].Hour()),      //nolint:gosec
					Minute:   api.SQLUSMALLINT(d[i].Minute()),    //nolint:gosec
					Second:   api.SQLUSMALLINT(d[i].Second()),    //nolint:gosec
					Fraction: api.SQLUINTEGER(d[i].Nanosecond()), //nolint:gosec
				}
			}
			p.Data = b
			buf = unsafe.Pointer(&b[0])
			sqltype = api.SQL_TYPE_TIMESTAMP
			if p.isDescribed && p.SQLType == api.SQL_TYPE_TIMESTAMP {
				decimal = p.Decimal
			}
			if decimal <= 0 {
				// represented as yyyy-mm-dd hh:mm:ss.fff format in ms sql server
				decimal = 3
			}
			size = 20 + api.SQLULEN(decimal)
		}
	default:
		panic(fmt.Errorf("unsupported bind param type %T", v))
	}
	ret := api.SQLBindParameter(h, odbcNum(idx),
		iotype, ctype, sqltype, size, decimal,
		api.SQLPOINTER(buf), buflen, plen)
	if IsError(ret) {
		return NewError("SQLBindParameter", h)
	}

	trc.Trace1("param.go: BindValue() - EXIT")
	return nil
}

// ExtractParameters will describe all the parameters.
func ExtractParameters(h api.SQLHSTMT, query ...string) ([]Parameter, error) {
	// count parameters
	trc.Trace1("param.go: ExtractParameters() - ENTRY")

	var n, nullable api.SQLSMALLINT
	ret := api.SQLNumParams(h, &n)
	if IsError(ret) {
		return nil, NewError("SQLNumParams", h)
	}
	if n <= 0 {
		// no parameters
		trc.Trace1("param.go: ExtractParameters() - no parameters")
		return nil, nil
	}
	ps := make([]Parameter, n)
	if runtime.GOOS == "zos" && len(query) > 0 && strings.HasPrefix(strings.TrimSpace(strings.ToUpper(query[0])), "SELECT") {
		trc.Trace1("param.go: ExtractParameters() - skipping SQLDescribeParam on z/OS")
		return ps, nil
	}
	// fetch param descriptions
	for i := range ps {
		p := &ps[i]
		ret = api.SQLDescribeParam(h, api.SQLUSMALLINT(i+1),
			&p.SQLType, &p.Size, &p.Decimal, &nullable)
		if IsError(ret) {
			// SQLDescribeParam is not implemented by freedts,
			// it even fails for some statements on windows.
			// Will try request without these descriptions
			continue
		}
		p.isDescribed = true
	}
	trc.Trace1("param.go: ExtractParameters() - EXIT")
	return ps, nil
}

// SqltoCtype function will convert the sql type to c type.
func SqltoCtype(sqltype api.SQLSMALLINT) api.SQLSMALLINT {
	trc.Trace1("param.go: SqltoCtype()")

	switch sqltype {
	case api.SQL_BIT:
		return api.SQL_C_BIT
	case api.SQL_TINYINT, api.SQL_SMALLINT, api.SQL_INTEGER:
		return api.SQL_C_LONG
	case api.SQL_BIGINT:
		return api.SQL_C_SBIGINT
	case api.SQL_NUMERIC, api.SQL_DECIMAL, api.SQL_FLOAT, api.SQL_REAL, api.SQL_DOUBLE:
		return api.SQL_C_DOUBLE
	case api.SQL_TYPE_TIMESTAMP:
		return api.SQL_C_TYPE_TIMESTAMP
	case api.SQL_TYPE_DATE:
		return api.SQL_C_TYPE_DATE
	case api.SQL_TYPE_TIME:
		return api.SQL_C_TYPE_TIME
	case api.SQL_CHAR, api.SQL_VARCHAR, api.SQL_CLOB, api.SQL_LONGVARCHAR:
		return api.SQL_C_CHAR
	case api.SQL_WCHAR, api.SQL_WVARCHAR, api.SQL_WLONGVARCHAR, api.SQL_SS_XML:
		return api.SQL_C_WCHAR
	case api.SQL_BINARY, api.SQL_VARBINARY, api.SQL_BLOB, api.SQL_LONGVARBINARY:
		return api.SQL_C_BINARY
	case api.SQL_DBCLOB:
		return api.SQL_C_DBCHAR
	default:
		panic(fmt.Errorf("unsupported param type %v at sql.out", sqltype))
	}
}
