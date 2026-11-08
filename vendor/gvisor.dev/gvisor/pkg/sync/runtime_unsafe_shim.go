// Shim for Go >= 1.21 — provides the same API as runtime_unsafe.go
// without the !go1.21 build constraint.

//go:build go1.21

package sync

import (
	"fmt"
	"reflect"
	"unsafe"
)

//go:nosplit
func Goyield() {
	goyield()
}

//go:nosplit
func Gopark(unlockf func(uintptr, unsafe.Pointer) bool, lock unsafe.Pointer, reason uint8, traceEv byte, traceskip int) {
	gopark(unlockf, lock, reason, traceEv, traceskip)
}

//go:linkname gopark runtime.gopark
func gopark(unlockf func(uintptr, unsafe.Pointer) bool, lock unsafe.Pointer, reason uint8, traceEv byte, traceskip int)

//go:linkname wakep runtime.wakep
func wakep()

//go:nosplit
func Wakep() {
	if supportsWakeSuppression {
		wakep()
	}
}

//go:linkname goready runtime.goready
func goready(gp uintptr, traceskip int)

//go:nosplit
func Goready(gp uintptr, traceskip int, wakep bool) {
	if supportsWakeSuppression && !wakep {
		preGoReadyWakeSuppression()
	}
	goready(gp, traceskip)
	if supportsWakeSuppression && !wakep {
		preGoReadyWakeSuppression()
	}
}

const (
	WaitReasonSelect      uint8 = 9
	WaitReasonChanReceive uint8 = 14
	WaitReasonSemacquire  uint8 = 18
)

const (
	TraceEvGoBlockRecv   byte = 23
	TraceEvGoBlockSelect byte = 24
	TraceEvGoBlockSync   byte = 25
)

func Rand32() uint32 {
	return fastrand()
}

func Rand64() uint64 {
	return uint64(fastrand())<<32 | uint64(fastrand())
}

//go:linkname fastrand runtime.fastrand
func fastrand() uint32

func RandUintptr() uintptr {
	if unsafe.Sizeof(uintptr(0)) == 4 {
		return uintptr(Rand32())
	}
	return uintptr(Rand64())
}

func MapKeyHasher(m interface{}) func(unsafe.Pointer, uintptr) uintptr {
	if rtyp := reflect.TypeOf(m); rtyp.Kind() != reflect.Map {
		panic(fmt.Sprintf("sync.MapKeyHasher: m is %v, not map", rtyp))
	}
	mtyp := *(**maptype)(unsafe.Pointer(&m))
	return mtyp.hasher
}

type maptype struct {
	size       uintptr
	ptrdata    uintptr
	hash       uint32
	tflag      uint8
	align      uint8
	fieldAlign uint8
	kind       uint8
	equal      func(unsafe.Pointer, unsafe.Pointer) bool
	gcdata     *byte
	str        int32
	ptrToThis  int32
	key        unsafe.Pointer
	elem       unsafe.Pointer
	bucket     unsafe.Pointer
	hasher     func(unsafe.Pointer, uintptr) uintptr
}

//go:linkname semacquire sync.runtime_Semacquire
func semacquire(addr *uint32)

//go:linkname semrelease sync.runtime_Semrelease
func semrelease(addr *uint32, handoff bool, skipframes int)

//go:linkname canSpin sync.runtime_canSpin
func canSpin(i int) bool

//go:linkname doSpin sync.runtime_doSpin
func doSpin()
