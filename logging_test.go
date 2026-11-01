package go_ibm_db

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ibmdb/go_ibm_db/api"
	trc "github.com/ibmdb/go_ibm_db/log2"
)

func TestDriverInitializationError(t *testing.T) {
	want := errors.New("CLI unavailable")
	conn, err := (&Driver{initErr: want}).Open("PWD=secret")
	if conn != nil || !errors.Is(err, want) {
		t.Fatalf("Open() = (%v, %v), want (nil, %v)", conn, err, want)
	}
}

func TestTraceDoesNotContainInput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.log")
	trc.GetPath(path)
	t.Cleanup(func() { trc.GetPath("") })

	api.StringToUTF16("PWD=secret")
	pool := Pconnect("PoolSize=1")
	pool.availablePool["PWD=secret"] = nil
	pool.Display()

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(contents), "secret") {
		t.Fatalf("trace contains connection credentials: %s", contents)
	}
}
