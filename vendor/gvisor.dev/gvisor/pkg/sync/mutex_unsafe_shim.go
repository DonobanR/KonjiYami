// Shim for Go >= 1.21 — provides the same API as mutex_unsafe.go
// without the !go1.21 build constraint.

//go:build go1.21

package sync

import (
	"sync"
	"sync/atomic"
	"unsafe"
)

type CrossGoroutineMutex struct {
	sync.Mutex
}

type syncMutex struct {
	state int32
	sema  uint32
}

func (m *CrossGoroutineMutex) state() *int32 {
	return &(*syncMutex)(unsafe.Pointer(&m.Mutex)).state
}

// +checklocksignore
func (m *CrossGoroutineMutex) Lock() {
	m.Mutex.Lock()
}

// +checklocksignore
func (m *CrossGoroutineMutex) Unlock() {
	m.Mutex.Unlock()
}

const (
	mutexUnlocked = 0
	mutexLocked   = 1
)

func (m *CrossGoroutineMutex) TryLock() bool {
	if atomic.CompareAndSwapInt32(m.state(), mutexUnlocked, mutexLocked) {
		if RaceEnabled {
			RaceAcquire(unsafe.Pointer(&m.Mutex))
		}
		return true
	}
	return false
}

type Mutex struct {
	m CrossGoroutineMutex
}

// +checklocksignore
func (m *Mutex) Lock() {
	noteLock(unsafe.Pointer(m))
	m.m.Lock()
}

// +checklocksignore
func (m *Mutex) Unlock() {
	noteUnlock(unsafe.Pointer(m))
	m.m.Unlock()
}

// +checklocksignore
func (m *Mutex) TryLock() bool {
	noteLock(unsafe.Pointer(m))
	locked := m.m.TryLock()
	if !locked {
		noteUnlock(unsafe.Pointer(m))
	}
	return locked
}
