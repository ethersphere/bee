package localstore

import "resenje.org/multex"

const (
	Upload   string = "upload"
	Reserve         = "reserve"
	GC              = "gc"
	Write           = "write"
	Sampling        = "sampling"
)

type storeLock struct {
	*multex.Multex
}

func newStoreLock() storeLock {
	return storeLock{
		Multex: multex.New(),
	}
}

func (s *storeLock) Lock(lk string) {
	switch lk {
	case Write:
		s.Multex.Lock(Reserve)
		s.Multex.Lock(GC)
	default:
		s.Multex.Lock(lk)
	}
}

func (s *storeLock) Unlock(lk string) {
	switch lk {
	case Write:
		s.Multex.Unlock(Reserve)
		s.Multex.Unlock(GC)
	default:
		s.Multex.Unlock(lk)
	}
}
