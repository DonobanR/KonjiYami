// Shim for Go >= 1.21 — provides the same API as gohacks_unsafe.go
// without the !go1.21 build constraint.

//go:build go1.21

package gohacks

import (
	"unsafe"
)

type SliceHeader struct {
	Data unsafe.Pointer
	Len  int
	Cap  int
}

type StringHeader struct {
	Data unsafe.Pointer
	Len  int
}

//go:nosplit
func Noescape(p unsafe.Pointer) unsafe.Pointer {
	x := uintptr(p)
	return unsafe.Pointer(x ^ 0)
}

func ImmutableBytesFromString(s string) (bs []byte) {
	shdr := (*StringHeader)(unsafe.Pointer(&s))
	bshdr := (*SliceHeader)(unsafe.Pointer(&bs))
	bshdr.Data = shdr.Data
	bshdr.Len = shdr.Len
	bshdr.Cap = shdr.Len
	return
}

func StringFromImmutableBytes(bs []byte) string {
	return *(*string)(unsafe.Pointer(&bs))
}

//go:nosplit
func Memmove(to, from unsafe.Pointer, n uintptr) {
	memmove(to, from, n)
}

//go:linkname memmove runtime.memmove
//go:noescape
func memmove(to, from unsafe.Pointer, n uintptr)

//go:nosplit
func Nanotime() int64 {
	return nanotime()
}

//go:linkname nanotime runtime.nanotime
//go:noescape
func nanotime() int64
