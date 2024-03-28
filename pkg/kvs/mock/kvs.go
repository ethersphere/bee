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
	memoryMock map[string]map[string][]byte
}

var singleInMemorySwarm *single

func getInMemorySwarm() *single {
	if singleInMemorySwarm == nil {
		lock.Lock()
		defer lock.Unlock()
		if singleInMemorySwarm == nil {
			singleInMemorySwarm = &single{
				memoryMock: make(map[string]map[string][]byte)}
		}
	}
	return singleInMemorySwarm
}

func getMemory() map[string]map[string][]byte {
	ch := make(chan *single)
	go func() {
		ch <- getInMemorySwarm()
	}()
	mem := <-ch
	return mem.memoryMock
}

type mockKeyValueStore struct {
	address swarm.Address
}

var _ kvs.KeyValueStore = (*mockKeyValueStore)(nil)

func (m *mockKeyValueStore) Get(key []byte) ([]byte, error) {
	mem := getMemory()
	val := mem[m.address.String()][hex.EncodeToString(key)]
	return val, nil
}

func (m *mockKeyValueStore) Put(key []byte, value []byte) error {
	mem := getMemory()
	if _, ok := mem[m.address.String()]; !ok {
		mem[m.address.String()] = make(map[string][]byte)
	}
	mem[m.address.String()][hex.EncodeToString(key)] = value
	return nil
}

func (m *mockKeyValueStore) Save() (swarm.Address, error) {
	return m.address, nil
}

func New() kvs.KeyValueStore {
	return &mockKeyValueStore{address: swarm.EmptyAddress}
}

func NewReference(address swarm.Address) kvs.KeyValueStore {
	return &mockKeyValueStore{address: address}
}
