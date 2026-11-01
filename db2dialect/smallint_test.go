package db2dialect

import (
	"math"
	"testing"
)

// TestSmallIntScanRange guards against the silent wraparound that used to
// turn out-of-range values into their int32 aliases: 2147483648 became
// -2147483648.
func TestSmallIntScanRange(t *testing.T) {
	t.Parallel()

	ok := []struct {
		name string
		val  any
		want SmallInt
	}{
		{"int64 max int32", int64(math.MaxInt32), SmallInt(math.MaxInt32)},
		{"int64 min int32", int64(math.MinInt32), SmallInt(math.MinInt32)},
		{"int max int32", int(math.MaxInt32), SmallInt(math.MaxInt32)},
		{"float64 in range", float64(12345), SmallInt(12345)},
		{"string in range", "32767", SmallInt(32767)},
		{"[]byte in range", []byte("-32768"), SmallInt(-32768)},
	}
	for _, tc := range ok {
		t.Run("ok/"+tc.name, func(t *testing.T) {
			t.Parallel()
			var s SmallInt
			if err := s.Scan(tc.val); err != nil {
				t.Fatalf("Scan(%v): %v", tc.val, err)
			}
			if s != tc.want {
				t.Fatalf("Scan(%v) = %d, want %d", tc.val, s, tc.want)
			}
		})
	}

	bad := []struct {
		name string
		val  any
	}{
		{"int64 above range", int64(math.MaxInt32) + 1},
		{"int64 below range", int64(math.MinInt32) - 1},
		{"int above range", int(math.MaxInt32) + 1},
		{"float64 above range", float64(math.MaxInt32) + 1},
		{"float64 NaN", math.NaN()},
		{"float64 +Inf", math.Inf(1)},
		{"string above range", "2147483648"},
		{"string below range", "-2147483649"},
	}
	for _, tc := range bad {
		t.Run("bad/"+tc.name, func(t *testing.T) {
			t.Parallel()
			var s SmallInt
			if err := s.Scan(tc.val); err == nil {
				t.Fatalf("Scan(%v) succeeded with %d, want range error", tc.val, s)
			}
		})
	}
}

// TestSmallIntBoolScanContract checks the 0/1 boolean contract and the
// range guard of SmallIntBool.
func TestSmallIntBoolScanContract(t *testing.T) {
	t.Parallel()

	ok := []struct {
		name string
		val  any
		want SmallIntBool
	}{
		{"int64 zero", int64(0), 0},
		{"int64 one", int64(1), 1},
		{"int one", int(1), 1},
		{"float64 zero", float64(0), 0},
		{"string true", "true", 1},
		{"string YES", "YES", 1},
		{"string no", "no", 0},
		{"string 0", "0", 0},
	}
	for _, tc := range ok {
		t.Run("ok/"+tc.name, func(t *testing.T) {
			t.Parallel()
			var s SmallIntBool
			if err := s.Scan(tc.val); err != nil {
				t.Fatalf("Scan(%v): %v", tc.val, err)
			}
			if s != tc.want {
				t.Fatalf("Scan(%v) = %d, want %d", tc.val, s, tc.want)
			}
		})
	}

	bad := []struct {
		name string
		val  any
	}{
		{"int64 seven", int64(7)},
		{"int32 minus one", int32(-1)},
		{"float64 two", float64(2)},
		{"string 2", "2"},
		{"int64 overflow", int64(math.MaxInt32) + 1},
	}
	for _, tc := range bad {
		t.Run("bad/"+tc.name, func(t *testing.T) {
			t.Parallel()
			var s SmallIntBool
			if err := s.Scan(tc.val); err == nil {
				t.Fatalf("Scan(%v) succeeded with %d, want contract error", tc.val, s)
			}
		})
	}
}

// TestNullSmallIntScanRange ensures the Null wrappers surface the same
// range errors instead of wrapping.
func TestNullSmallIntScanRange(t *testing.T) {
	t.Parallel()

	var n NullSmallInt
	if err := n.Scan(int64(math.MaxInt32) + 1); err == nil {
		t.Fatalf("NullSmallInt.Scan above range succeeded: %+v", n)
	}
	var nb NullSmallIntBool
	if err := nb.Scan(int64(5)); err == nil {
		t.Fatalf("NullSmallIntBool.Scan(5) succeeded: %+v", nb)
	}
}
