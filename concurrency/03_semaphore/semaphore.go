package semaphore

import (
	"primitives/internal/futex"
	"strconv"
	"sync/atomic"
)

// 32bit: 2^31-1; 64bit: 2^32-1
const MAX_PERMITS = min(uint64(1<<32-1), uint64(^uint(0)>>1))

type Semaphore struct {
	permits    uint32
	capability uint32
}

func New(n int) *Semaphore {
	// only uint32
	if n < 0 {
		panic("permits number invalid")
	}
	if n > int(MAX_PERMITS) {
		// there is error before this chehck in 32 bit system
		panic("permits number is greater than maximum " + strconv.FormatUint(MAX_PERMITS, 10))
	}
	return &Semaphore{uint32(n), uint32(n)}
}

func (s *Semaphore) Acquire() {
	for !s.TryAcquire() {
		futex.Wait(&s.permits, 0)
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
		if permits := atomic.LoadUint32(&s.permits); uint64(permits) == MAX_PERMITS {
			panic("overflow permits number")
		} else if atomic.CompareAndSwapUint32(&s.permits, permits, permits+1) {
			futex.Wake(&s.permits)
			return
		}
	}
}

func (s *Semaphore) Available() int {
	return int(atomic.LoadUint32(&s.permits))
}
