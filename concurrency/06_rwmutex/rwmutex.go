package rwmutex

import (
	"primitives/internal/futex"
	"sync/atomic"
)

const writer = 1 << 31

type RWMutex struct {
	state uint32
}

func (rw *RWMutex) RLock() {
	for {
		if state := atomic.LoadUint32(&rw.state); state&writer != 0 {
			futex.Wait(&rw.state, state)

			continue
		} else if state == writer-1 {
			panic("too many read")
		} else if atomic.CompareAndSwapUint32(&rw.state, state, state+1) {
			return
		}
	}
}

func (rw *RWMutex) RUnlock() {
	for {
		if state := atomic.LoadUint32(&rw.state); state&(writer-1) == 0 {
			panic("runlock runlocked")
		} else if nstate := state - 1; atomic.CompareAndSwapUint32(&rw.state, state, nstate) {
			if nstate == writer {
				futex.WakeAll(&rw.state)
			}

			return
		}
	}
}

func (rw *RWMutex) Lock() {
	for {
		if state := atomic.LoadUint32(&rw.state); state&writer != 0 {
			futex.Wait(&rw.state, state)

			continue
		} else if atomic.CompareAndSwapUint32(&rw.state, state, state|writer) {
			break
		}
	}

	for {
		if state := atomic.LoadUint32(&rw.state); state == writer {
			return
		} else {
			futex.Wait(&rw.state, state)
		}
	}
}

func (rw *RWMutex) Unlock() {
	if !atomic.CompareAndSwapUint32(&rw.state, writer, 0) {
		panic("unlock unlocked")
	}
	futex.WakeAll(&rw.state)
}
