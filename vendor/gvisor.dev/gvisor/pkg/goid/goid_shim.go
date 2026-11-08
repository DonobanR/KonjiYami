// Shim for Go >= 1.21 — provides the same API as goid.go
// without the !go1.21 build constraint.

//go:build go1.21

package goid

type stack struct {
	lo uintptr
	hi uintptr
}

type gobuf struct {
	sp   uintptr
	pc   uintptr
	g    uintptr
	ctxt uintptr
	ret  uint64
	lr   uintptr
	bp   uintptr
}

type g struct {
	stack       stack
	stackguard0 uintptr
	stackguard1 uintptr

	_panic       uintptr
	_defer       uintptr
	m            uintptr
	sched        gobuf
	syscallsp    uintptr
	syscallpc    uintptr
	stktopsp     uintptr
	param        uintptr
	atomicstatus uint32
	stackLock    uint32
	goid         int64
}

// getg is defined in assembly (goid_amd64.s).
func getg() *g

// Get returns the ID of the current goroutine.
func Get() int64 {
	return getg().goid
}
