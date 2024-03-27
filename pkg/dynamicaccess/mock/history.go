package mock

import (
	"context"
	"sort"
	"time"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/feeds"
	"github.com/ethersphere/bee/pkg/kvs"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

type historyMock struct {
	history map[int64]kvs.KeyValueStore
}

func NewHistory() *historyMock {
	return &historyMock{history: make(map[int64]kvs.KeyValueStore)}
}

func (h *historyMock) Add(timestamp int64, act kvs.KeyValueStore) error {
	h.history[timestamp] = act
	return nil
}

func (h *historyMock) Insert(timestamp int64, act kvs.KeyValueStore) *historyMock {
	h.Add(timestamp, act)
	return h
}

func (h *historyMock) Lookup(at int64) (kvs.KeyValueStore, error) {
	keys := []int64{}
	for k := range h.history {
		keys = append(keys, k)
	}

	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	timestamp := time.Now()
	if at != 0 {
		timestamp = time.Unix(at, 0)
	}

	for i := len(keys) - 1; i >= 0; i-- {
		update := time.Unix(keys[i], 0)
		if update.Before(timestamp) || update.Equal(timestamp) {
			return h.history[keys[i]], nil
		}
	}
	return nil, nil
}

func (h *historyMock) Get(timestamp int64) (kvs.KeyValueStore, error) {
	return h.history[timestamp], nil
}

type finder struct {
	getter *feeds.Getter
}

type updater struct {
	*feeds.Putter
	next uint64
}

func (f *finder) At(ctx context.Context, at int64, after uint64) (chunk swarm.Chunk, currentIndex, nextIndex feeds.Index, err error) {
	return nil, nil, nil, nil
}

func HistoryFinder(getter storage.Getter, feed *feeds.Feed) feeds.Lookup {
	return &finder{feeds.NewGetter(getter, feed)}
}

func (u *updater) Update(ctx context.Context, at int64, payload []byte) error {
	return nil
}

func (u *updater) Feed() *feeds.Feed {
	return nil
}

func HistoryUpdater(putter storage.Putter, signer crypto.Signer, topic []byte) (feeds.Updater, error) {
	p, err := feeds.NewPutter(putter, signer, topic)
	if err != nil {
		return nil, err
	}
	return &updater{Putter: p}, nil
}
