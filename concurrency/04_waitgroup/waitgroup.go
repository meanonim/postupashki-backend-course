package waitgroup

import (
	"math"
	"primitives/internal/futex"
	"sync/atomic"
)

type WaitGroup struct {
	count uint32
}

func (wg *WaitGroup) Add(delta int) {
	for {
		if c := atomic.LoadUint32(&wg.count); -int64(c) > int64(delta) {
			panic("wg negative")
		} else if int64(math.MaxUint32-c) < int64(delta) {
			panic("wg overflow")
		} else if cn := c + uint32(delta); atomic.CompareAndSwapUint32(&wg.count, c, cn) {
			// wake all if we set 0
			if c != 0 && cn == 0 {
				futex.WakeAll(&wg.count)
			}
			return
		}
	}
}

func (wg *WaitGroup) Done() {
	wg.Add(-1)
}

func (wg *WaitGroup) Wait() {
	for {
		if c := atomic.LoadUint32(&wg.count); c == 0 {
			return
		} else {
			futex.Wait(&wg.count, c)

		}
	}
}
