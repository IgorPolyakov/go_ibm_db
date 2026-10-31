package main

import "testing"

func TestFloatArray(t *testing.T) {
	SkipOnPlatform(t, PlatformInformix) // Informix lacks the DB2 types/syntax this test needs
	if FloatArray() != nil {
		t.Error("Error at FloatArray")
	}
}
