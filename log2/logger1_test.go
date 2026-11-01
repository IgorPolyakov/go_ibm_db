package log2

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type countedStringer struct{ calls int }

func (s *countedStringer) String() string {
	s.calls++
	return "formatted"
}

func TestTraceFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.log")
	if err := os.WriteFile(path, []byte("previous\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	globalOutput := log.Writer()
	GetPath(path)
	t.Cleanup(func() { GetPath("") })
	Trace1("operation=%s", "prepare")
	Trace1("returned %d", -1)

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"previous", "operation=prepare", "returned -1"} {
		if !strings.Contains(string(contents), want) {
			t.Errorf("trace lacks %q: %s", want, contents)
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("trace permissions = %o, want 600", info.Mode().Perm())
	}
	if log.Writer() != globalOutput {
		t.Error("trace changed the default logger output")
	}
}

func TestTraceDisabledAndConcurrent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.log")
	GetPath("")
	t.Cleanup(func() { GetPath("") })
	value := &countedStringer{}
	Trace1("%s", value)
	if value.calls != 0 {
		t.Fatal("formatted a disabled trace")
	}

	var group sync.WaitGroup
	group.Add(2)
	go func() {
		defer group.Done()
		for range 100 {
			GetPath(path)
			GetPath("")
		}
	}()
	go func() {
		defer group.Done()
		for range 1000 {
			Trace1("operation=%d", 1)
		}
	}()
	group.Wait()
}
