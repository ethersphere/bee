package dynamicaccess

import "crypto/ecdsa"

type GranteeManager interface {
	Get(topic string) []*ecdsa.PublicKey
	Add(topic string, addList []*ecdsa.PublicKey) error
	Publish(act Act, publisher ecdsa.PublicKey, topic string) Act

	// HandleGrantees(topic string, addList, removeList []*ecdsa.PublicKey) *Act

	// Load(grantee Grantee)
	// Save()
}

var _ GranteeManager = (*granteeManager)(nil)

type granteeManager struct {
	accessLogic AccessLogic
	granteeList Grantee
}

func NewGranteeManager(al AccessLogic) *granteeManager {
	return &granteeManager{accessLogic: al, granteeList: NewGrantee()}
}

func (gm *granteeManager) Get(topic string) []*ecdsa.PublicKey {
	return gm.granteeList.GetGrantees(topic)
}

func (gm *granteeManager) Add(topic string, addList []*ecdsa.PublicKey) error {
	return gm.granteeList.AddGrantees(topic, addList)
}

func (gm *granteeManager) Publish(act Act, publisher ecdsa.PublicKey, topic string) Act {
	gm.accessLogic.AddPublisher(act, publisher, "")
	for _, grantee := range gm.granteeList.GetGrantees(topic) {
		gm.accessLogic.Add_New_Grantee_To_Content(act, publisher, *grantee)
	}
	return act
}
