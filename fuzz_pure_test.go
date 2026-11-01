package go_ibm_db

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/ibmdb/go_ibm_db/api"
)

// FuzzMaskDSN checks that DSN masking never panics and never leaks the
// password value for any input shape (spaces around keys, case, empty
// segments, repeated separators).
func FuzzMaskDSN(f *testing.F) {
	f.Add("DATABASE=x;UID=u;PWD=secret")
	f.Add("PWD =secret")
	f.Add("pwd=")
	f.Add(";;;PWD=a;;PWD=b")
	f.Add("PWD===x")
	f.Fuzz(func(t *testing.T, dsn string) {
		out := maskDSN(dsn)

		// maskDSN replaces segments in place: the segment count is
		// preserved, so every PWD-like segment must come back masked
		in := strings.Split(dsn, ";")
		got := strings.Split(out, ";")
		if len(in) != len(got) {
			t.Fatalf("dsn %q: segment count changed: %q", dsn, out)
		}
		for i, p := range in {
			kv := strings.SplitN(p, "=", 2)
			if len(kv) != 2 || !strings.EqualFold(strings.TrimSpace(kv[0]), "PWD") {
				continue
			}
			if !strings.HasSuffix(got[i], "=***") {
				t.Fatalf("dsn %q: segment %d not masked: %q (full: %q)", dsn, i, got[i], out)
			}
		}
	})
}

// FuzzParseDSN checks that DSN parsing never panics, always reports a
// positive fetch size and strips the FETCHSIZE/ROWARRAYSIZE directives
// from the clean DSN it returns.
func FuzzParseDSN(f *testing.F) {
	f.Add("HOSTNAME=h;PORT=1;DATABASE=d;UID=u;PWD=p;FETCHSIZE=100")
	f.Add("ROWARRAYSIZE=abc")
	f.Add("FETCHSIZE=-5")
	f.Add(";;;")
	f.Fuzz(func(t *testing.T, dsn string) {
		clean, fetchSize := parseDSN(dsn)
		if fetchSize < 1 {
			t.Fatalf("dsn %q: fetchSize %d < 1", dsn, fetchSize)
		}
		for _, key := range []string{"FETCHSIZE", "ROWARRAYSIZE"} {
			for _, part := range strings.Split(clean, ";") {
				kv := strings.SplitN(part, "=", 2)
				if len(kv) == 2 && strings.EqualFold(strings.TrimSpace(kv[0]), key) {
					t.Fatalf("dsn %q: clean DSN %q still contains %s", dsn, clean, key)
				}
			}
		}
	})
}

// FuzzUTF16RoundTrip checks the UTF-16 <-> UTF-8 helpers used for every
// character value crossing the ODBC boundary: valid UTF-8 must survive
// the round trip exactly, and arbitrary UTF-16 input must not panic.
func FuzzUTF16RoundTrip(f *testing.F) {
	f.Add("hello")
	f.Add("привет мир")
	f.Add("日本語テキスト")
	f.Add("emoji 🚀🚀")
	f.Add("")
	f.Fuzz(func(t *testing.T, s string) {
		if !utf8.ValidString(s) || strings.ContainsRune(s, 0) {
			// NUL is the ODBC string terminator: encoding is fine, but
			// the decoder intentionally stops at the first NUL
			return
		}
		encoded := api.StringToUTF16(s)
		decoded := utf16toutf8(encoded)
		if string(decoded) != s {
			t.Fatalf("round trip changed %q into %q", s, string(decoded))
		}
	})
}

// FuzzUTF16Arbitrary feeds arbitrary UTF-16 code units (including lone
// surrogates) into the decoder: it must not panic or index out of range.
func FuzzUTF16Arbitrary(f *testing.F) {
	f.Add([]byte{0x00, 0x41, 0x00, 0x00})
	f.Add([]byte{0xd8, 0x00, 0xdc, 0x00, 0x00, 0x00})
	f.Add([]byte{0xd8, 0x00})
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, raw []byte) {
		units := make([]uint16, len(raw)/2)
		for i := range units {
			units[i] = uint16(raw[2*i])<<8 | uint16(raw[2*i+1])
		}
		_ = utf16toutf8(units)
		_ = dbclobToUTF8(raw)
	})
}

// FuzzConvertAssign drives the scan conversion with arbitrary destination
// types and driver values: it must not panic, and same-type assignments
// must be exact.
func FuzzConvertAssign(f *testing.F) {
	f.Add(int64(42), "text")
	f.Add(int64(0), "")
	f.Add(int64(-7), "héllo")
	f.Add(int64(1<<40), "日本語")
	f.Fuzz(func(t *testing.T, num int64, text string) {
		values := []any{
			int64(num), float64(num), text, []byte(text), true, nil,
			time.Unix(num%1_000_000_000, 0),
		}
		for _, src := range values {
			var (
				i  int64
				fl float64
				st string
			)
			if err := ConvertAssign(&i, src); err == nil {
				if v, ok := src.(int64); ok && i != v {
					t.Fatalf("int64 dest: got %d want %d", i, v)
				}
			}
			if err := ConvertAssign(&fl, src); err == nil {
				if v, ok := src.(float64); ok && fl != v {
					t.Fatalf("float64 dest: got %f want %f", fl, v)
				}
			}
			if err := ConvertAssign(&st, src); err == nil {
				if v, ok := src.(string); ok && st != v {
					t.Fatalf("string dest: got %q want %q", st, v)
				}
			}
		}
	})
}
