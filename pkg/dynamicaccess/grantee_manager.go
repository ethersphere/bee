package dynamicaccess

import (
	"crypto/ecdsa"

	"github.com/ethersphere/bee/pkg/swarm"
)

type GranteeManager interface {
	Get(topic string) []*ecdsa.PublicKey
	Add(topic string, addList []*ecdsa.PublicKey) error
	Publish(rootHash swarm.Address, publisher *ecdsa.PublicKey, topic string) (swarm.Address, error)

	// HandleGrantees(topic string, addList, removeList []*ecdsa.PublicKey) *Act

	// Load(grantee Grantee)
	// Save()
}

var _ GranteeManager = (*granteeManager)(nil)

type granteeManager struct {
	accessLogic ActLogic
	granteeList Grantee
}

func NewGranteeManager(al ActLogic) *granteeManager {
	return &granteeManager{accessLogic: al, granteeList: NewGrantee()}
}

func (gm *granteeManager) Get(topic string) []*ecdsa.PublicKey {
	return gm.granteeList.GetGrantees(topic)
}

func (gm *granteeManager) Add(topic string, addList []*ecdsa.PublicKey) error {
	return gm.granteeList.AddGrantees(topic, addList)
}

func (gm *granteeManager) Publish(rootHash swarm.Address, publisher *ecdsa.PublicKey, topic string) (swarm.Address, error) {
	ref, err := gm.accessLogic.AddPublisher(rootHash, publisher)
	for _, grantee := range gm.granteeList.GetGrantees(topic) {
		ref, err = gm.accessLogic.AddNewGranteeToContent(ref, publisher, grantee)
	}
	return ref, err
}
