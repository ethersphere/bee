package dynamicaccess

import (
	"crypto/ecdsa"

	"github.com/ethersphere/bee/pkg/dynamicaccess/mock"
	"github.com/ethersphere/bee/pkg/kvs"
	"github.com/ethersphere/bee/pkg/swarm"
)

type GranteeManager interface {
	Get() []*ecdsa.PublicKey
	Add(addList []*ecdsa.PublicKey) error
	Publish(kvs kvs.KeyValueStore, publisher *ecdsa.PublicKey) (swarm.Address, error)

	// HandleGrantees(addList, removeList []*ecdsa.PublicKey) *Act

	// Load(grantee Grantee)
	// Save()
}

var _ GranteeManager = (*granteeManager)(nil)

type granteeManager struct {
	accessLogic ActLogic
	granteeList *mock.GranteeListStructMock
}

func NewGranteeManager(al ActLogic) *granteeManager {
	return &granteeManager{accessLogic: al, granteeList: mock.NewGranteeList()}
}

func (gm *granteeManager) Get() []*ecdsa.PublicKey {
	return gm.granteeList.Get()
}

func (gm *granteeManager) Add(addList []*ecdsa.PublicKey) error {
	return gm.granteeList.Add(addList)
}

func (gm *granteeManager) Publish(kvs kvs.KeyValueStore, publisher *ecdsa.PublicKey) (swarm.Address, error) {
	err := gm.accessLogic.AddPublisher(kvs, publisher)
	for _, grantee := range gm.granteeList.Get() {
		err = gm.accessLogic.AddGrantee(kvs, publisher, grantee, nil)
	}
	return swarm.EmptyAddress, err
}
