package semaphore

import (
	"math"
	"primitives/internal/futex"
	"strconv"
	"sync/atomic"
)

const maxPermits = min(math.MaxUint32, math.MaxInt)

type Semaphore struct {
	permits uint32
	waiters atomic.Uint32
}

func New(n int) *Semaphore {
	// only uint32
	if n < 0 {
		panic("permits number invalid")
	}
	if n > int(maxPermits) {
		// there is error before this chehck in 32 bit system
		panic("permits number is greater than maximum " + strconv.FormatUint(maxPermits, 10))
	}
	return &Semaphore{
		permits: uint32(n),
	}
}

func (s *Semaphore) Acquire() {
	for !s.TryAcquire() {
		s.waiters.Add(1)
		futex.Wait(&s.permits, 0)
		s.waiters.Add(^uint32(0))
	}
}

func (s *Semaphore) TryAcquire() bool {
	for {
		if permits := atomic.LoadUint32(&s.permits); permits == 0 {
			return false
		} else if atomic.CompareAndSwapUint32(&s.permits, permits, permits-1) {
			return true
		}
	}
}

func (s *Semaphore) Release() {
	for {
		// release can increase counter to 'infinity'
		if permits := atomic.LoadUint32(&s.permits); uint64(permits) == maxPermits {
			panic("overflow permits number")
		} else if atomic.CompareAndSwapUint32(&s.permits, permits, permits+1) {

			if s.waiters.Load() > 0 {
				futex.Wake(&s.permits)
			}
			return
		}
	}
}

func (s *Semaphore) Available() int {
	return int(atomic.LoadUint32(&s.permits))
}
