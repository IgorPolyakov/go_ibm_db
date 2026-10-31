package go_ibm_db

import (
	"errors"
	"fmt"
	"strings"
	"unsafe"

	"github.com/ibmdb/go_ibm_db/api"
	trc "github.com/ibmdb/go_ibm_db/log2"
)

// CreateDb function will take the db name and user details as parameters
// and create the database.
func CreateDb(dbname string, connStr string, options ...string) (bool, error) {
	trc.Trace1("database.go: CreateDb() - ENTRY")
	trc.Trace1(fmt.Sprintf("dbname=%s, connStr=%s", dbname, maskDSN(connStr)))

	if dbname == "" {
		trc.Trace1("Error: Database name cannot be empty")
		trc.Trace1("database.go: CreateDb() - EXIT")
		return false, errors.New("database name cannot be empty")
	}
	var codeset, mode string
	count := len(options)
	if count > 0 {
		for i := range count {
			opt := strings.Split(options[i], "=")
			switch opt[0] {
			case "codeset":
				codeset = opt[1]
			case "mode":
				mode = opt[1]
			default:
				return false, errors.New("not a valid parameter")
			}
		}
	}
	connStr = connStr + ";" + "ATTACH=true"
	trc.Trace1("database.go: CreateDb() - EXIT")
	return createDatabase(dbname, connStr, codeset, mode)
}

func createDatabase(dbname string, connStr string, codeset string, mode string) (bool, error) {
	trc.Trace1("database.go: createDatabase() - ENTRY")
	trc.Trace1(fmt.Sprintf("dbname=%s, connStr=%s, codeset=%s, mode=%s", dbname, maskDSN(connStr), codeset, mode))
	var out api.SQLHANDLE
	in := api.SQLHANDLE(api.SQL_NULL_HANDLE)
	bufDBN := api.StringToUTF16(dbname)
	bufCS := api.StringToUTF16(connStr)
	bufC := api.StringToUTF16(codeset)
	bufM := api.StringToUTF16(mode)

	ret := api.SQLAllocHandle(api.SQL_HANDLE_ENV, in, &out)
	if IsError(ret) {
		return false, NewError("SQLAllocHandle", api.SQLHENV(in))
	}
	drvH := api.SQLHENV(out)
	ret = api.SQLAllocHandle(api.SQL_HANDLE_DBC, api.SQLHANDLE(drvH), &out)
	if IsError(ret) {
		defer func() { _ = releaseHandle(drvH) }()
		return false, NewError("SQLAllocHandle", drvH)
	}
	hdbc := api.SQLHDBC(out)
	ret = api.SQLDriverConnect(hdbc, 0,
		(*api.SQLWCHAR)(unsafe.Pointer(&bufCS[0])), api.SQLSMALLINT(len(bufCS)),
		nil, 0, nil, api.SQL_DRIVER_NOPROMPT)
	if IsError(ret) {
		defer func() { _ = releaseHandle(hdbc) }()
		return false, NewError("SQLDriverConnect", hdbc)
	}
	switch {
	case codeset == "" && mode == "":
		ret = api.SQLCreateDb(hdbc, (*api.SQLWCHAR)(unsafe.Pointer(&bufDBN[0])), api.SQLINTEGER(len(bufDBN)), nil, 0, nil, 0)
	case codeset == "":
		ret = api.SQLCreateDb(hdbc, (*api.SQLWCHAR)(unsafe.Pointer(&bufDBN[0])), api.SQLINTEGER(len(bufDBN)), nil, 0, (*api.SQLWCHAR)(unsafe.Pointer(&bufM[0])), api.SQLINTEGER(len(bufM)))
	case mode == "":
		ret = api.SQLCreateDb(hdbc, (*api.SQLWCHAR)(unsafe.Pointer(&bufDBN[0])), api.SQLINTEGER(len(bufDBN)), (*api.SQLWCHAR)(unsafe.Pointer(&bufC[0])), api.SQLINTEGER(len(bufC)), nil, 0)
	default:
		ret = api.SQLCreateDb(hdbc, (*api.SQLWCHAR)(unsafe.Pointer(&bufDBN[0])), api.SQLINTEGER(len(bufDBN)), (*api.SQLWCHAR)(unsafe.Pointer(&bufC[0])), api.SQLINTEGER(len(bufC)), (*api.SQLWCHAR)(unsafe.Pointer(&bufM[0])), api.SQLINTEGER(len(bufM)))
	}
	if IsError(ret) {
		defer func() { _ = releaseHandle(hdbc) }()
		return false, NewError("SQLCreateDb", hdbc)
	}
	defer func() { _ = releaseHandle(hdbc) }()

	trc.Trace1("database.go: createDatabase() - EXIT")
	return true, nil
}

// DropDb function will take the db name and user details as parameters
// and drop the database.
func DropDb(dbname string, connStr string) (bool, error) {
	trc.Trace1("database.go: DropDb() - ENTRY")
	trc.Trace1(fmt.Sprintf("dbname=%s, connStr=%s", dbname, maskDSN(connStr)))

	if dbname == "" {
		return false, errors.New("database name cannot be empty")
	}
	connStr = connStr + ";" + "ATTACH=true"
	trc.Trace1("database.go: DropDb() - EXIT")
	return dropDatabase(dbname, connStr)
}

func dropDatabase(dbname string, connStr string) (bool, error) {
	trc.Trace1("database.go: dropDatabase() - ENTRY")
	trc.Trace1(fmt.Sprintf("dbname=%s, connStr=%s", dbname, maskDSN(connStr)))

	var out api.SQLHANDLE
	in := api.SQLHANDLE(api.SQL_NULL_HANDLE)
	bufDBN := api.StringToUTF16(dbname)
	bufCS := api.StringToUTF16(connStr)

	ret := api.SQLAllocHandle(api.SQL_HANDLE_ENV, in, &out)
	if IsError(ret) {
		return false, NewError("SQLAllocHandle", api.SQLHENV(in))
	}
	drvH := api.SQLHENV(out)
	ret = api.SQLAllocHandle(api.SQL_HANDLE_DBC, api.SQLHANDLE(drvH), &out)
	if IsError(ret) {
		defer func() { _ = releaseHandle(drvH) }()
		return false, NewError("SQLAllocHandle", drvH)
	}
	hdbc := api.SQLHDBC(out)
	ret = api.SQLDriverConnect(hdbc, 0,
		(*api.SQLWCHAR)(unsafe.Pointer(&bufCS[0])), api.SQLSMALLINT(len(bufCS)),
		nil, 0, nil, api.SQL_DRIVER_NOPROMPT)
	if IsError(ret) {
		defer func() { _ = releaseHandle(hdbc) }()
		return false, NewError("SQLDriverConnect", hdbc)
	}
	ret = api.SQLDropDb(hdbc, (*api.SQLWCHAR)(unsafe.Pointer(&bufDBN[0])), api.SQLINTEGER(len(bufDBN)))
	if IsError(ret) {
		defer func() { _ = releaseHandle(hdbc) }()
		return false, NewError("SQLDropDb", hdbc)
	}
	defer func() { _ = releaseHandle(hdbc) }()
	return true, nil
}
