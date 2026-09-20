package once

import (
	"primitives/internal/futex"
	"sync/atomic"
)

const (
	none = iota
	run
	done
)

type Once struct {
	state uint32
}

func (o *Once) Do(f func()) {
	if o.Done() {
		return
	}

	if atomic.CompareAndSwapUint32(&o.state, none, run) {
		// defer for case if f panics
		defer func() {
			atomic.StoreUint32(&o.state, done)
			futex.WakeAll(&o.state)
		}()

		f()
		return
	}

	for !o.Done() {
		futex.Wait(&o.state, run)
	}
}

func (o *Once) Done() bool {
	return atomic.LoadUint32(&o.state) == done
}
