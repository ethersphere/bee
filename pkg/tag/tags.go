package tag

import (
	"errors"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	TagNotFoundErr = errors.New("tag not found")
)

// Tags hold tag information indexed by a unique random uint32
type Tags struct {
	tags *sync.Map
}

// NewTags creates a tags object
func NewTags() *Tags {
	return &Tags{
		tags: &sync.Map{},
	}
}

// Create creates a new tag, stores it by the name and returns it
// it returns an error if the tag with this name already exists
func (ts *Tags) Create() (uint32, error) {
	t := NewTag()
	if _, loaded := ts.tags.LoadOrStore(t.Uid, t); loaded {
		return 0, errExists
	}
	return t.Uid, nil
}

// Get returns the underlying tag for the uid or an error if not found
func (ts *Tags) Get(uid uint32) (*Tag, error) {
	t, ok := ts.tags.Load(uid)
	if !ok {
		return nil, TagNotFoundErr
	}
	return t.(*Tag), nil
}

// GetByAddress returns the latest underlying tag for the address or an error if not found
func (ts *Tags) GetByAddress(address swarm.Address) (*Tag, error) {
	var t *Tag
	var lastTime time.Time
	ts.tags.Range(func(key interface{}, value interface{}) bool {
		rcvdTag := value.(*Tag)
		if swarm.Address.Equal(address, rcvdTag.Address) && rcvdTag.StartedAt.After(lastTime) {
			t = rcvdTag
			lastTime = rcvdTag.StartedAt
		}
		return true
	})
	if t == nil {
		return nil, errTagNotFound
	}
	return t, nil
}
