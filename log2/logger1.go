// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package log2 provides a minimal file/stdout tracer used by the driver
// for optional diagnostics (enabled via log2.GetPath).
package log2

import (
	"fmt"
	"log"
	"os"
)

var globalvar string = ""

// traceFileWarned prevents spamming stderr when the trace file stays broken.
var traceFileWarned bool

// GetPath selects the trace destination: a file path (the file is
// recreated on each run), "stdout", or empty to disable tracing.
func GetPath(filename string) {
	// fmt.Println("filename = ", filename)
	// fmt.Println("Args Length = ", argsLen)

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

// Trace1 writes one message to the configured trace destination; it is
// a no-op when tracing is disabled and never terminates the program.
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
