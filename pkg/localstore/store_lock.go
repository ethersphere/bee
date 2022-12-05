package localstore

import "resenje.org/multex"

const (
	UploadLock  string = "upload"
	ReserveLock        = "reserve"
	GCLock             = "gc"
	Write              = "write"
)

type storeLock struct {
	multex.Multex
}

func (s *storeLock) Lock(lk string) {
	switch lk {
	case UploadLock, ReserveLock, GCLock:
		s.Multex.Lock(lk)
	case Write:
		s.Multex.Lock(ReserveLock)
		s.Multex.Lock(GCLock)
	}
}

func (s *storeLock) Unlock(lk string) {
	switch lk {
	case UploadLock, ReserveLock, GCLock:
		s.Multex.Unlock(lk)
	case Write:
		s.Multex.Unlock(ReserveLock)
		s.Multex.Unlock(GCLock)
	}
}
