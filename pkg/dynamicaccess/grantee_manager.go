package dynamicaccess

import (
	"crypto/ecdsa"

	"github.com/ethersphere/bee/pkg/kvs"
	"github.com/ethersphere/bee/pkg/swarm"
)

type GranteeManager interface {
	Get(topic string) []*ecdsa.PublicKey
	Add(topic string, addList []*ecdsa.PublicKey) error
	Publish(kvs kvs.KeyValueStore, publisher *ecdsa.PublicKey, topic string) (swarm.Address, error)

	// HandleGrantees(topic string, addList, removeList []*ecdsa.PublicKey) *Act

	// Load(grantee Grantee)
	// Save()
}

var _ GranteeManager = (*granteeManager)(nil)

type granteeManager struct {
	accessLogic ActLogic
	granteeList GranteeList
}

func NewGranteeManager(al ActLogic) *granteeManager {
	return &granteeManager{accessLogic: al, granteeList: NewGrantee()}
}

func (gm *granteeManager) Get(topic string) []*ecdsa.PublicKey {
	return gm.granteeList.Get(topic)
}

func (gm *granteeManager) Add(topic string, addList []*ecdsa.PublicKey) error {
	return gm.granteeList.Add(topic, addList)
}

func (gm *granteeManager) Publish(kvs kvs.KeyValueStore, publisher *ecdsa.PublicKey, topic string) (swarm.Address, error) {
	err := gm.accessLogic.AddPublisher(kvs, publisher)
	for _, grantee := range gm.granteeList.Get(topic) {
		err = gm.accessLogic.AddGrantee(kvs, publisher, grantee, nil)
	}
	return swarm.EmptyAddress, err
}
