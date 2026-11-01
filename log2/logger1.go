// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package log2 provides a minimal file/stdout tracer used by the driver
// for optional diagnostics (enabled via log2.GetPath).
package log2

import (
	"log"
	"os"
	"sync"
)

var trace struct {
	sync.Mutex
	logger *log.Logger
	file   *os.File
}

// GetPath selects the trace destination: a file path, "stdout", or empty
// to disable tracing. File output is appended with owner-only permissions.
func GetPath(filename string) {
	trace.Lock()
	defer trace.Unlock()

	trace.logger = nil
	if trace.file != nil {
		_ = trace.file.Close()
		trace.file = nil
	}

	switch filename {
	case "":
		return
	case "stdout":
		trace.logger = log.New(os.Stdout, "", log.LstdFlags)
		return
	}

	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	if err = file.Chmod(0o600); err != nil {
		_ = file.Close()
		return
	}
	trace.file = file
	trace.logger = log.New(file, "", log.LstdFlags)
}

// Trace1 writes one message to the configured trace destination. Formatting
// happens only when tracing is enabled.
func Trace1(msg string, args ...any) {
	trace.Lock()
	defer trace.Unlock()

	if trace.logger == nil {
		return
	}
	if len(args) == 0 {
		trace.logger.Println(msg)
	} else {
		trace.logger.Printf(msg, args...)
	}
}
