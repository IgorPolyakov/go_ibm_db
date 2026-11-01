//go:build !lockorder

package go_ibm_db

import "sync"

// mutex is an alias of sync.Mutex in normal builds; compile with
// -tags lockorder to swap in a lock-order-tracking implementation that
// detects inconsistent acquisition orders across the driver's mutexes.
type mutex = sync.Mutex
