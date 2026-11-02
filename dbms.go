package go_ibm_db

import "strings"

// dbmsDialect holds server-specific SQL used by the core driver.
type dbmsDialect interface {
	lastInsertIDQuery() string
}

type db2DBMS struct{}

func (db2DBMS) lastInsertIDQuery() string {
	return "select identity_val_local() as last_inserted_id from sysibm.sysdummy1"
}

type informixDBMS struct{}

func (informixDBMS) lastInsertIDQuery() string {
	return "select DBINFO('sqlca.sqlerrd1') from systables where tabid = 1"
}

func dbmsForName(name string) dbmsDialect {
	switch {
	case strings.HasPrefix(strings.ToUpper(name), "IDS/"):
		return informixDBMS{}
	default:
		return db2DBMS{}
	}
}
