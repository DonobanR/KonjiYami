// Shim for Go >= 1.21 — provides the same API as runtime_amd64.go
// without the !go1.21 build constraint.

//go:build amd64 && go1.21 && !goexperiment.staticlockranking

package sync

import (
	"sync/atomic"
)

const supportsWakeSuppression = true

func addrOfSpinning() *int32

var nmspinning = addrOfSpinning()

//go:nosplit
func preGoReadyWakeSuppression() {
	atomic.AddInt32(nmspinning, 1)
}

//go:nosplit
func postGoReadyWakeSuppression() {
	atomic.AddInt32(nmspinning, -1)
}
