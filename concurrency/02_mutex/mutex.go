package mutex

import (
	"primitives/internal/futex"
	"runtime"
	"sync/atomic"
)

const (
	free = iota
	held
	contended
)

type Mutex struct {
	state uint32
}

func (m *Mutex) Lock() {
	// fast way
	if m.TryLock() {
		return
	}

	// fast-to-slow way
	// wait while state is held
	for i := range 64 {
		if state := atomic.LoadUint32(&m.state); state == contended {
			// someone waits lock too, no need to go further in cycle
			break
		} else if state == free && m.TryLock() {
			// mutex is free to use, try to lock it asap
			return
		}

		if i%16 == 0 {
			runtime.Gosched()
		}
	}

	// slow way
	// contented or held state here

	// wait up to state is free, try to catch the mutex
	// current goroutine fights with other goroutines
	for atomic.SwapUint32(&m.state, contended) != free {
		// set contended and just sleep
		futex.Wait(&m.state, contended)
	}

}

func (m *Mutex) TryLock() bool {
	return atomic.CompareAndSwapUint32(&m.state, free, held)
}

func (m *Mutex) Unlock() {
	if oldState := atomic.SwapUint32(&m.state, free); oldState == free {
		panic("unlock of unlocked mutex")
	} else if oldState == contended {
		futex.Wake(&m.state)
	}
	// if it was held, so no need to wake someone
}
