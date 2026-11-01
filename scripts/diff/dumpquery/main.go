// Command dumpquery prints the result of a query with | separated values
// (NULL as \N) so it can be diffed against another client's output.
//
// go run ./scripts/diff/dumpquery -dsn "$DB2_STRESS_DSN" -q "SELECT ..."
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"time"

	_ "github.com/ibmdb/go_ibm_db"
)

func main() {
	dsn := flag.String("dsn", "", "connection string")
	query := flag.String("q", "", "query to run")
	flag.Parse()
	if *dsn == "" || *query == "" {
		fmt.Fprintln(os.Stderr, "usage: dumpquery -dsn DSN -q QUERY")
		os.Exit(2)
	}
	db, err := sql.Open("go_ibm_db", *dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open:", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()
	rows, err := db.Query(*query)
	if err != nil {
		fmt.Fprintln(os.Stderr, "query:", err)
		os.Exit(1)
	}
	defer func() { _ = rows.Close() }()
	cols, err := rows.Columns()
	if err != nil {
		fmt.Fprintln(os.Stderr, "columns:", err)
		os.Exit(1)
	}
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			fmt.Fprintln(os.Stderr, "scan:", err)
			os.Exit(1)
		}
		for i, v := range vals {
			if i > 0 {
				fmt.Print("|")
			}
			switch x := v.(type) {
			case nil:
				fmt.Print("\\N")
			case []byte:
				fmt.Print(string(x))
			case time.Time:
				fmt.Print(x.Format("2006-01-02 15:04:05.000000"))
			default:
				fmt.Print(x)
			}
		}
		fmt.Println()
	}
	if err := rows.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "rows:", err)
		os.Exit(1)
	}
}
