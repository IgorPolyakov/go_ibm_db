package main

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestEmptyArrayParameterErrors verifies that binding an empty array
// parameter returns an error instead of panicking. All five supported
// array types are covered; before the fix each panicked with an index out
// of range while deriving the binding from d[0].
func TestEmptyArrayParameterErrors(t *testing.T) {
	SkipOnPlatform(t, PlatformZOS)

	db := Createconnection()
	defer db.Close()

	db.Exec("DROP TABLE goemptyarr")
	if _, err := db.Exec("CREATE TABLE goemptyarr (id INTEGER NOT NULL PRIMARY KEY)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	defer db.Exec("DROP TABLE goemptyarr")

	ctx := context.Background()
	query := "INSERT INTO goemptyarr VALUES (?)"
	emptyArrays := []struct {
		name string
		arg  any
	}{
		{"[]int64", []int64{}},
		{"[]string", []string{}},
		{"[]bool", []bool{}},
		{"[]float64", []float64{}},
		{"[]time.Time", []time.Time{}},
	}

	for _, tc := range emptyArrays {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%s: panicked: %v", tc.name, r)
				}
			}()
			_, err := db.ExecContext(ctx, query, tc.arg)
			if err == nil {
				t.Errorf("%s: empty array insert unexpectedly succeeded", tc.name)
				return
			}
			fmt.Printf("%s: %v\n", tc.name, err)
		}()
	}
}
