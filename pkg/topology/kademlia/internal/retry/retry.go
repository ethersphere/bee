package retry

import (
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/swarm"
)

type counter struct {
	tryAfter       time.Time
	failedAttempts int
}

type Retry struct {
	info map[string]*counter
	sync.Mutex
}

func New() *Retry {
	return &Retry{
		info: make(map[string]*counter),
	}
}

func (r *Retry) Set(addr swarm.Address, next time.Time, attempts int) {

	r.Lock()
	defer r.Unlock()

	if info, ok := r.info[addr.ByteString()]; ok {
		info.tryAfter = next
		info.failedAttempts = attempts
	} else {
		r.info[addr.ByteString()] = &counter{tryAfter: next, failedAttempts: attempts}
	}
}

func (r *Retry) Allow(addr swarm.Address) bool {

	r.Lock()
	defer r.Unlock()

	if info, ok := r.info[addr.ByteString()]; ok && time.Now().Before(info.tryAfter) {
		return false
	}

	return true
}

func (r *Retry) Attempts(addr swarm.Address) int {

	r.Lock()
	defer r.Unlock()

	if info, ok := r.info[addr.ByteString()]; ok && time.Now().Before(info.tryAfter) {
		return info.failedAttempts
	}

	return 0
}

func (r *Retry) Remove(addr swarm.Address) {

	r.Lock()
	defer r.Unlock()

	delete(r.info, addr.ByteString())
}
