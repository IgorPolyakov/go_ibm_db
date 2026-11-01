// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build zos

package api

import (
	"fmt"
	"unsafe"

	trc "github.com/ibmdb/go_ibm_db/log2"
	"github.com/ibmruntimes/go-recordio/v2/utils"
)

var dll utils.Dll
var loadError error

func init() {
	if err := dll.Open("DSNAO64C"); err != nil {
		loadError = fmt.Errorf("load DB2 CLI DLL DSNAO64C: %w", err)
	}
}

// SQLAllocHandle allocates an environment, connection or statement handle.
func SQLAllocHandle(handleType SQLSMALLINT, inputHandle SQLHANDLE, outputHandle *SQLHANDLE) SQLRETURN {
	trc.Trace1("api/zapi_zos.go SQLAllocHandle() - ENTRY")
	trc.Trace1("handleType = %d", handleType)

	r := utils.CfuncEbcdic(getFunc(&dll, "SQLAllocHandle"), uintptr(handleType), uintptr(inputHandle), uintptr(unsafe.Pointer(outputHandle)))

	trc.Trace1("r = %d", r)
	trc.Trace1("api/zapi_zos.go SQLAllocHandle() - EXIT")
	return SQLRETURN(r)
}

// SQLBindCol binds a result-set column to an application buffer.
func SQLBindCol(statementHandle SQLHSTMT, columnNumber SQLUSMALLINT, targetType SQLSMALLINT, targetValuePtr []byte, bufferLength SQLLEN, vallen *SQLLEN) SQLRETURN {
	trc.Trace1("api/zapi_zos.go SQLBindCol() - ENTRY")
	trc.Trace1("columnNumber=%d, targetType=%d, bufferLength=%d", columnNumber, targetType, bufferLength)

	r := utils.CfuncEbcdic(getFunc(&dll, "SQLBindCol"), uintptr(statementHandle), uintptr(columnNumber), uintptr(targetType), uintptr(unsafe.Pointer(&targetValuePtr[0])), uintptr(bufferLength), uintptr(unsafe.Pointer(vallen)))

	trc.Trace1("api/zapi_zos.go SQLBindCol() - EXIT")
	return SQLRETURN(r)
}

// SQLBindParameter binds a parameter marker to an application buffer.
func SQLBindParameter(statementHandle SQLHSTMT, parameterNumber SQLUSMALLINT, inputOutputType SQLSMALLINT, valueType SQLSMALLINT, parameterType SQLSMALLINT, columnSize SQLULEN, decimalDigits SQLSMALLINT, parameterValue SQLPOINTER, bufferLength SQLLEN, ind *SQLLEN) SQLRETURN {
	trc.Trace1("api/zapi_zos.go SQLBindParameter() - ENTRY")

	r := utils.CfuncEbcdic(getFunc(&dll, "SQLBindParameter"), uintptr(statementHandle), uintptr(parameterNumber), uintptr(inputOutputType), uintptr(valueType), uintptr(parameterType), uintptr(columnSize), uintptr(decimalDigits), uintptr(parameterValue), uintptr(bufferLength), uintptr(unsafe.Pointer(ind)))

	trc.Trace1("api/zapi_zos.go SQLBindParameter() - EXIT")
	return SQLRETURN(r)
}

// SQLCancel cancels an in-flight statement execution or fetch; it is safe to call from another thread.
func SQLCancel(statementHandle SQLHSTMT) SQLRETURN {
	trc.Trace1("api/zapi_zos.go SQLCancel() - ENTRY")

	r := utils.CfuncEbcdic(getFunc(&dll, "SQLCancel"), uintptr(statementHandle))

	trc.Trace1("api/zapi_zos.go SQLCancel() - EXIT")
	return SQLRETURN(r)
}

// SQLCloseCursor closes the open cursor of a statement.
func SQLCloseCursor(statementHandle SQLHSTMT) SQLRETURN {
	trc.Trace1("api/zapi_zos.go SQLCloseCursor() - ENTRY")

	r := utils.CfuncEbcdic(getFunc(&dll, "SQLCloseCursor"), uintptr(statementHandle))

	trc.Trace1("api/zapi_zos.go SQLCloseCursor() - EXIT")
	return SQLRETURN(r)
}

// SQLDescribeCol returns the description of a result-set column.
func SQLDescribeCol(statementHandle SQLHSTMT, columnNumber SQLUSMALLINT, columnName *SQLWCHAR, bufferLength SQLSMALLINT, nameLengthPtr *SQLSMALLINT, dataTypePtr *SQLSMALLINT, columnSizePtr *SQLULEN, decimalDigitsPtr *SQLSMALLINT, nullablePtr *SQLSMALLINT) SQLRETURN {
	trc.Trace1("api/zapi_zos.go SQLDescribeCol() - ENTRY")

	r := utils.CfuncEbcdic(getFunc(&dll, "SQLDescribeColW"), uintptr(statementHandle), uintptr(columnNumber), uintptr(unsafe.Pointer(columnName)), uintptr(bufferLength), uintptr(unsafe.Pointer(nameLengthPtr)), uintptr(unsafe.Pointer(dataTypePtr)), uintptr(unsafe.Pointer(columnSizePtr)), uintptr(unsafe.Pointer(decimalDigitsPtr)), uintptr(unsafe.Pointer(nullablePtr)))

	trc.Trace1("api/zapi_zos.go SQLDescribeCol() - EXIT")
	return SQLRETURN(r)
}

// SQLDescribeParam returns the description of a parameter marker.
func SQLDescribeParam(statementHandle SQLHSTMT, parameterNumber SQLUSMALLINT, dataTypePtr *SQLSMALLINT, parameterSizePtr *SQLULEN, decimalDigitsPtr *SQLSMALLINT, nullablePtr *SQLSMALLINT) SQLRETURN {
	trc.Trace1("api/zapi_zos.go SQLDescribeParam() - ENTRY")

	r := utils.CfuncEbcdic(getFunc(&dll, "SQLDescribeParam"), uintptr(statementHandle), uintptr(parameterNumber), uintptr(unsafe.Pointer(dataTypePtr)), uintptr(unsafe.Pointer(parameterSizePtr)), uintptr(unsafe.Pointer(decimalDigitsPtr)), uintptr(unsafe.Pointer(nullablePtr)))

	trc.Trace1("api/zapi_zos.go SQLDescribeParam() - EXIT")
	return SQLRETURN(r)
}

// SQLDisconnect closes the connection.
func SQLDisconnect(connectionHandle SQLHDBC) SQLRETURN {
	trc.Trace1("api/zapi_zos.go SQLDisconnect() - ENTRY")

	r := utils.CfuncEbcdic(getFunc(&dll, "SQLDisconnect"), uintptr(connectionHandle))

	trc.Trace1("api/zapi_zos.go SQLDisconnect() - EXIT")
	return SQLRETURN(r)
}

// SQLDriverConnect opens a connection from a connection string.
func SQLDriverConnect(connectionHandle SQLHDBC, windowHandle SQLHWND, inConnectionString *SQLWCHAR, stringLength1 SQLSMALLINT, outConnectionString *SQLWCHAR, bufferLength SQLSMALLINT, stringLength2Ptr *SQLSMALLINT, driverCompletion SQLUSMALLINT) SQLRETURN {
	trc.Trace1("api/zapi_zos.go SQLDriverConnect() - ENTRY")

	r := utils.CfuncEbcdic(getFunc(&dll, "SQLDriverConnectW"), uintptr(connectionHandle), uintptr(windowHandle), uintptr(unsafe.Pointer(inConnectionString)), uintptr(stringLength1), uintptr(unsafe.Pointer(outConnectionString)), uintptr(bufferLength), uintptr(unsafe.Pointer(stringLength2Ptr)), uintptr(driverCompletion))

	trc.Trace1("api/zapi_zos.go SQLDriverConnect() - EXIT")
	return SQLRETURN(r)
}

// SQLEndTran commits or rolls back a transaction.
func SQLEndTran(handleType SQLSMALLINT, handle SQLHANDLE, completionType SQLSMALLINT) SQLRETURN {
	trc.Trace1("api/zapi_zos.go SQLEndTran() - ENTRY")

	r := utils.CfuncEbcdic(getFunc(&dll, "SQLEndTran"), uintptr(handleType), uintptr(handle), uintptr(completionType))

	trc.Trace1("api/zapi_zos.go SQLEndTran() - EXIT")
	return SQLRETURN(r)
}

// SQLExecute executes a prepared statement.
func SQLExecute(statementHandle SQLHSTMT) SQLRETURN {
	trc.Trace1("api/zapi_zos.go SQLExecute() - ENTRY")

	r := utils.CfuncEbcdic(getFunc(&dll, "SQLExecute"), uintptr(statementHandle))

	trc.Trace1("api/zapi_zos.go SQLExecute() - EXIT")
	return SQLRETURN(r)
}

// SQLFetch fetches the next row (or row block) of the result set.
func SQLFetch(statementHandle SQLHSTMT) SQLRETURN {
	trc.Trace1("api/zapi_zos.go SQLFetch() - ENTRY")

	r := utils.CfuncEbcdic(getFunc(&dll, "SQLFetch"), uintptr(statementHandle))

	trc.Trace1("api/zapi_zos.go SQLFetch() - EXIT")
	return SQLRETURN(r)
}

// SQLFreeHandle frees an environment, connection or statement handle.
func SQLFreeHandle(handleType SQLSMALLINT, handle SQLHANDLE) SQLRETURN {
	trc.Trace1("api/zapi_zos.go SQLFreeHandle() - ENTRY")

	r := utils.CfuncEbcdic(getFunc(&dll, "SQLFreeHandle"), uintptr(handleType), uintptr(handle))

	trc.Trace1("api/zapi_zos.go SQLFreeHandle() - EXIT")
	return SQLRETURN(r)
}

// SQLGetData retrieves data for a single unbound column.
func SQLGetData(statementHandle SQLHSTMT, colOrParamNum SQLUSMALLINT, targetType SQLSMALLINT, targetValuePtr SQLPOINTER, bufferLength SQLLEN, vallen *SQLLEN) SQLRETURN {
	trc.Trace1("api/zapi_zos.go SQLGetData() - ENTRY")

	r := utils.CfuncEbcdic(getFunc(&dll, "SQLGetData"), uintptr(statementHandle), uintptr(colOrParamNum), uintptr(targetType), uintptr(targetValuePtr), uintptr(bufferLength), uintptr(unsafe.Pointer(vallen)))

	trc.Trace1("api/zapi_zos.go SQLGetData() - EXIT")
	return SQLRETURN(r)
}

// SQLGetDiagRec returns one diagnostic record of a handle.
func SQLGetDiagRec(handleType SQLSMALLINT, handle SQLHANDLE, recNumber SQLSMALLINT, sqlState *SQLWCHAR, nativeErrorPtr *SQLINTEGER, messageText *SQLWCHAR, bufferLength SQLSMALLINT, textLengthPtr *SQLSMALLINT) SQLRETURN {
	trc.Trace1("api/zapi_zos.go SQLGetDiagRec() - ENTRY")

	r := utils.CfuncEbcdic(getFunc(&dll, "SQLGetDiagRecW"), uintptr(handleType), uintptr(handle), uintptr(recNumber), uintptr(unsafe.Pointer(sqlState)), uintptr(unsafe.Pointer(nativeErrorPtr)), uintptr(unsafe.Pointer(messageText)), uintptr(bufferLength), uintptr(unsafe.Pointer(textLengthPtr)))

	trc.Trace1("api/zapi_zos.go SQLGetDiagRec() - EXIT")
	return SQLRETURN(r)
}

// SQLGetInfo returns DBMS/driver information as a string or number.
func SQLGetInfo(connectionHandle SQLHDBC, infoType SQLUSMALLINT, infoValuePtr SQLPOINTER, bufferLength SQLSMALLINT, stringLengthPtr *SQLSMALLINT) SQLRETURN {
	trc.Trace1("api/zapi_zos.go SQLGetInfo() - ENTRY")

	r := utils.CfuncEbcdic(getFunc(&dll, "SQLGetInfoW"), uintptr(connectionHandle), uintptr(infoType), uintptr(infoValuePtr), uintptr(bufferLength), uintptr(unsafe.Pointer(stringLengthPtr)))

	trc.Trace1("api/zapi_zos.go SQLGetInfo() - EXIT")
	return SQLRETURN(r)
}

// SQLNumParams returns the number of parameter markers of a statement.
func SQLNumParams(statementHandle SQLHSTMT, parameterCountPtr *SQLSMALLINT) SQLRETURN {
	trc.Trace1("api/zapi_zos.go SQLNumParams() - ENTRY")

	r := utils.CfuncEbcdic(getFunc(&dll, "SQLNumParams"), uintptr(statementHandle), uintptr(unsafe.Pointer(parameterCountPtr)))

	trc.Trace1("api/zapi_zos.go SQLNumParams() - EXIT")
	return SQLRETURN(r)
}

// SQLNumResultCols returns the number of result-set columns of a statement.
func SQLNumResultCols(statementHandle SQLHSTMT, columnCountPtr *SQLSMALLINT) SQLRETURN {
	trc.Trace1("api/zapi_zos.go SQLNumResultCols() - ENTRY")

	r := utils.CfuncEbcdic(getFunc(&dll, "SQLNumResultCols"), uintptr(statementHandle), uintptr(unsafe.Pointer(columnCountPtr)))

	trc.Trace1("api/zapi_zos.go SQLNumResultCols() - EXIT")
	return SQLRETURN(r)
}

// SQLPrepare prepares an SQL string for execution.
func SQLPrepare(statementHandle SQLHSTMT, statementText *SQLWCHAR, textLength SQLINTEGER) SQLRETURN {
	trc.Trace1("api/zapi_zos.go SQLPrepare() - ENTRY")

	r := utils.CfuncEbcdic(getFunc(&dll, "SQLPrepareW"), uintptr(statementHandle), uintptr(unsafe.Pointer(statementText)), uintptr(textLength))

	trc.Trace1("api/zapi_zos.go SQLPrepare() - EXIT")
	return SQLRETURN(r)
}

// SQLRowCount returns the number of rows affected by the statement.
func SQLRowCount(statementHandle SQLHSTMT, rowCountPtr *SQLLEN) SQLRETURN {
	trc.Trace1("api/zapi_zos.go SQLRowCount() - ENTRY")

	r := utils.CfuncEbcdic(getFunc(&dll, "SQLRowCount"), uintptr(statementHandle), uintptr(unsafe.Pointer(rowCountPtr)))

	trc.Trace1("api/zapi_zos.go SQLRowCount() - EXIT")
	return SQLRETURN(r)
}

// SQLSetEnvAttr sets an environment attribute.
//
// Deprecated: kept for backward compatibility only.
func SQLSetEnvAttr(environmentHandle SQLHENV, attribute SQLINTEGER, valuePtr SQLPOINTER, stringLength SQLINTEGER) SQLRETURN {
	trc.Trace1("api/zapi_zos.go SQLSetEnvAttr() - ENTRY")

	r := utils.CfuncEbcdic(getFunc(&dll, "SQLSetEnvAttr"), uintptr(environmentHandle), uintptr(attribute), uintptr(valuePtr), uintptr(stringLength))

	trc.Trace1("api/zapi_zos.go SQLSetEnvAttr() - EXIT")
	return SQLRETURN(r)
}

// SQLSetConnectAttr sets a connection attribute.
//
// Deprecated: kept for backward compatibility only.
func SQLSetConnectAttr(connectionHandle SQLHDBC, attribute SQLINTEGER, valuePtr SQLPOINTER, stringLength SQLINTEGER) SQLRETURN {
	trc.Trace1("api/zapi_zos.go SQLSetConnectAttr() - ENTRY")

	r := utils.CfuncEbcdic(getFunc(&dll, "SQLSetConnectAttrW"), uintptr(connectionHandle), uintptr(attribute), uintptr(valuePtr), uintptr(stringLength))

	trc.Trace1("api/zapi_zos.go SQLSetConnectAttr() - EXIT")
	return SQLRETURN(r)
}

// SQLColAttribute returns a column descriptor field.
func SQLColAttribute(statementHandle SQLHSTMT, columnNumber SQLUSMALLINT, fieldIdentifier SQLUSMALLINT, characterAttributePtr SQLPOINTER, bufferLength SQLSMALLINT, stringLengthPtr *SQLSMALLINT, numericAttributePtr SQLPOINTER) SQLRETURN {
	trc.Trace1("api/zapi_zos.go SQLColAttribute() - ENTRY")

	r := utils.CfuncEbcdic(getFunc(&dll, "SQLColAttribute"), uintptr(statementHandle), uintptr(columnNumber), uintptr(fieldIdentifier), uintptr(characterAttributePtr), uintptr(bufferLength), uintptr(unsafe.Pointer(stringLengthPtr)), uintptr(numericAttributePtr))

	trc.Trace1("api/zapi_zos.go SQLColAttribute() - EXIT")
	return SQLRETURN(r)
}

// SQLMoreResults advances to the next result set of a batch.
func SQLMoreResults(statementHandle SQLHSTMT) SQLRETURN {
	trc.Trace1("api/zapi_zos.go SQLMoreResults() - ENTRY")

	r := utils.CfuncEbcdic(getFunc(&dll, "SQLMoreResults"), uintptr(statementHandle))

	trc.Trace1("api/zapi_zos.go SQLMoreResults() - EXIT")
	return SQLRETURN(r)
}

// SQLSetStmtAttr sets a statement attribute.
func SQLSetStmtAttr(statementHandle SQLHSTMT, attribute SQLINTEGER, valuePtr SQLPOINTER, stringLength SQLINTEGER) SQLRETURN {
	trc.Trace1("api/zapi_zos.go SQLSetStmtAttr() - ENTRY")

	r := utils.CfuncEbcdic(getFunc(&dll, "SQLSetStmtAttr"), uintptr(statementHandle), uintptr(attribute), uintptr(valuePtr), uintptr(stringLength))

	trc.Trace1("api/zapi_zos.go SQLSetStmtAttr() - EXIT")
	return SQLRETURN(r)
}

// SQLCreateDb creates a database (DB2 CLI extension).
func SQLCreateDb(connectionHandle SQLHDBC, dbnamePtr *SQLWCHAR, dbnameLen SQLINTEGER, codeSetPtr *SQLWCHAR, codeSetLen SQLINTEGER, modePtr *SQLWCHAR, modeLen SQLINTEGER) SQLRETURN {
	trc.Trace1("api/zapi_zos.go SQLCreateDb() - ENTRY")

	r := utils.CfuncEbcdic(getFunc(&dll, "SQLCreateDbW"), uintptr(connectionHandle), uintptr(unsafe.Pointer(dbnamePtr)), uintptr(dbnameLen), uintptr(unsafe.Pointer(codeSetPtr)), uintptr(codeSetLen), uintptr(unsafe.Pointer(modePtr)), uintptr(modeLen))

	trc.Trace1("api/zapi_zos.go SQLCreateDb() - EXIT")
	return SQLRETURN(r)
}

// SQLDropDb drops a database (DB2 CLI extension).
func SQLDropDb(connectionHandle SQLHDBC, dbnamePtr *SQLWCHAR, dbnameLen SQLINTEGER) SQLRETURN {
	trc.Trace1("api/zapi_zos.go SQLDropDb() - ENTRY")

	r := utils.CfuncEbcdic(getFunc(&dll, "SQLDropDbW"), uintptr(connectionHandle), uintptr(unsafe.Pointer(dbnamePtr)), uintptr(dbnameLen))

	trc.Trace1("api/zapi_zos.go SQLDropDb() - EXIT")
	return SQLRETURN(r)
}

// SQLExecDirect prepares and executes a statement in one call.
func SQLExecDirect(statementHandle SQLHSTMT, statementText *SQLWCHAR, textLength SQLINTEGER) SQLRETURN {
	trc.Trace1("api/zapi_zos.go SQLExecDirect() - ENTRY")

	r := utils.CfuncEbcdic(getFunc(&dll, "SQLExecDirectW"), uintptr(statementHandle), uintptr(unsafe.Pointer(statementText)), uintptr(textLength))

	trc.Trace1("api/zapi_zos.go SQLExecDirect() - EXIT")
	return SQLRETURN(r)
}
