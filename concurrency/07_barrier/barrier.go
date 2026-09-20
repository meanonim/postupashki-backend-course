package barrier

import (
	"math"
	"primitives/internal/futex"
	"sync/atomic"
)

type Barrier struct {
	need    uint32
	arrived uint32
	round   uint32
}

func New(n int) *Barrier {
	if n <= 0 || uint64(n) > math.MaxUint32 {
		panic("invalid need count")
	}
	return &Barrier{need: uint32(n), arrived: 0, round: 0}
}

func (b *Barrier) Wait() {
	if b.need <= 0 {
		panic("invalid need count, must be > 0")
	}

	r := atomic.LoadUint32(&b.round)
	ar := atomic.AddUint32(&b.arrived, 1)
	if ar == b.need {
		atomic.StoreUint32(&b.arrived, 0)
		atomic.AddUint32(&b.round, 1)

		futex.WakeAll(&b.round)
		return
	}

	for r == atomic.LoadUint32(&b.round) {
		futex.Wait(&b.round, r)
	}
}
