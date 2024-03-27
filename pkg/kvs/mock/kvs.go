package mock

import (
	"encoding/hex"
	"sync"

	"github.com/ethersphere/bee/pkg/kvs"
	"github.com/ethersphere/bee/pkg/swarm"
)

var lock = &sync.Mutex{}

type single struct {
	// TODO string -> []byte ?
	memoryMock map[string][]byte
}

var singleInMemorySwarm *single

func getInMemorySwarm() *single {
	if singleInMemorySwarm == nil {
		lock.Lock()
		defer lock.Unlock()
		if singleInMemorySwarm == nil {
			singleInMemorySwarm = &single{
				memoryMock: make(map[string][]byte)}
		}
	}
	return singleInMemorySwarm
}

func getMemory() map[string][]byte {
	ch := make(chan *single)
	go func() {
		ch <- getInMemorySwarm()
	}()
	mem := <-ch
	return mem.memoryMock
}

type mockKeyValueStore struct {
}

var _ kvs.KeyValueStore = (*mockKeyValueStore)(nil)

func (m *mockKeyValueStore) Get(key []byte) ([]byte, error) {
	mem := getMemory()
	val := mem[hex.EncodeToString(key)]
	return val, nil
}

func (m *mockKeyValueStore) Put(key []byte, value []byte) error {
	mem := getMemory()
	mem[hex.EncodeToString(key)] = value
	return nil
}

func (s *mockKeyValueStore) Save() (swarm.Address, error) {
	return swarm.ZeroAddress, nil
}

func New() kvs.KeyValueStore {
	return &mockKeyValueStore{}
}
