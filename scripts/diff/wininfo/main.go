//go:build windows

// Diagnostic: dump SQLGetInfo results for several info types.
// GOOS=windows go build -o wininfo.exe ./scripts/diff/wininfo
package main

import (
	"fmt"
	"os"
	"unsafe"

	"github.com/ibmdb/go_ibm_db/api"
)

func main() {
	dsn := os.Getenv("DB2_STRESS_DSN")
	if dsn == "" {
		fmt.Println("set DB2_STRESS_DSN")
		os.Exit(2)
	}
	var env api.SQLHENV
	if api.SQLAllocHandle(api.SQL_HANDLE_ENV, api.SQLHANDLE(0), (*api.SQLHANDLE)(&env)) != api.SQL_SUCCESS {
		fmt.Println("alloc env failed")
		os.Exit(1)
	}
	defer api.SQLFreeHandle(api.SQL_HANDLE_ENV, api.SQLHANDLE(env))
	var dbc api.SQLHDBC
	if api.SQLAllocHandle(api.SQL_HANDLE_DBC, api.SQLHANDLE(env), (*api.SQLHANDLE)(&dbc)) != api.SQL_SUCCESS {
		fmt.Println("alloc dbc failed")
		os.Exit(1)
	}
	b := api.StringToUTF16(dsn)
	ret := api.SQLDriverConnect(dbc, 0,
		(*api.SQLWCHAR)(unsafe.Pointer(&b[0])), api.SQLSMALLINT(len(b)),
		nil, 0, nil, api.SQL_DRIVER_NOPROMPT)
	fmt.Printf("connect ret=%d\n", ret)
	if ret != api.SQL_SUCCESS && ret != api.SQL_SUCCESS_WITH_INFO {
		os.Exit(1)
	}
	defer api.SQLDisconnect(dbc)

	// scenario mimicking the driver: run a query first, then GetInfo
	// on the same connection
	var stmt api.SQLHSTMT
	if api.SQLAllocHandle(api.SQL_HANDLE_STMT, api.SQLHANDLE(dbc), (*api.SQLHANDLE)(&stmt)) == api.SQL_SUCCESS {
		q := api.StringToUTF16("SELECT COUNT(*) FROM sysdatabases")
		api.SQLExecDirectW(stmt, (*api.SQLWCHAR)(unsafe.Pointer(&q[0])), api.SQL_NTS) //nolint:errcheck
		api.SQLFetch(stmt)                                                            //nolint:errcheck
		api.SQLFreeHandle(api.SQL_HANDLE_STMT, api.SQLHANDLE(stmt))                   //nolint:errcheck
	}

	types := map[string]api.SQLUSMALLINT{
		"SQL_DBMS_NAME":   api.SQL_DBMS_NAME,
		"SQL_DBMS_VER":    api.SQL_DBMS_VER,
		"SQL_DRIVER_NAME": api.SQL_DRIVER_NAME,
		"SQL_DRIVER_VER":  api.SQL_DRIVER_VER,
		"SQL_SERVER_NAME": api.SQL_SERVER_NAME,
	}
	for _, name := range []string{"SQL_DBMS_NAME", "SQL_DBMS_VER", "SQL_DRIVER_NAME", "SQL_DRIVER_VER", "SQL_SERVER_NAME"} {
		buf := make([]api.SQLWCHAR, 128)
		var outLen api.SQLSMALLINT
		ret := api.SQLGetInfo(dbc, types[name],
			api.SQLPOINTER(unsafe.Pointer(&buf[0])), api.SQLSMALLINT(2*len(buf)), &outLen)
		fmt.Printf("%s: ret=%d outLen=%d hex=% x\n", name, ret, outLen, buf16bytes(buf, 16))
	}
}

func buf16bytes(buf []api.SQLWCHAR, max int) []byte {
	out := []byte{}
	for i := 0; i < max && i < len(buf); i++ {
		out = append(out, byte(buf[i]), byte(buf[i]>>8))
	}
	return out
}
