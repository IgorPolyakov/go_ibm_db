// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package odbc implements database/sql driver to access data via odbc interface.
package log2

import (
	"fmt"
	"log"
	"os"
)

var globalvar string = ""

// traceFileWarned prevents spamming stderr when the trace file stays broken.
var traceFileWarned bool

func GetPath(filename string) {
	// fmt.Println("filename = ", filename)
	//fmt.Println("Args Length = ", argsLen)

	if filename != "" && filename != "stdout" {
		if _, err := os.Stat(filename); err == nil {
			// fmt.Println("File exits\n")
			e := os.Remove(filename)
			if e != nil {
				fmt.Println("Problem in removing existing log file")
			}
		}
	}

	globalvar = filename
}

func Trace1(msg1 string) {
	if globalvar == "" {
		return
	}

	if globalvar == "stdout" {
		log.SetOutput(os.Stdout)
		log.Println(msg1)
		return
	}

	// file, errlog := os.OpenFile("C:\\temp\\testlogs2.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	file, errlog := os.OpenFile(globalvar, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if errlog != nil {
		// a broken trace file must not kill the application
		if !traceFileWarned {
			traceFileWarned = true
			fmt.Fprintln(os.Stderr, "go_ibm_db: cannot open trace file:", errlog)
		}
		return
	}
	defer func() { _ = file.Close() }()

	log.SetOutput(file)
	log.Println(msg1)

	_ = file.Sync()
}
