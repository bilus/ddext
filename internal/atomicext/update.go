package atomicext

import (
	"errors"
)

type Number interface {
	~uint32 | ~int32 | ~uint64 | ~int64 | ~float32 | ~float64
}

type Atomic[T any] interface {
	Load() T
	CompareAndSwap(old T, new T) bool
}

func Update[T Number](a Atomic[T], retries int, f func(old T) T) error {
	// Store max gauge value, retrying if another thread modified it.
	for {
		old := a.Load()
		value := f(old)
		if old < value {
			if a.CompareAndSwap(old, value) {
				return nil
			}
			retries--
			if retries == 0 {
				return errors.New("Timed out trying to update an atomic value")
			}
		}
		return nil
	}
}
