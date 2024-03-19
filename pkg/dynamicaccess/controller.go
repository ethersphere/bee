package dynamicaccess

import (
	"crypto/ecdsa"

	"github.com/ethersphere/bee/pkg/swarm"
)

type Controller interface {
	DownloadHandler(timestamp int64, enryptedRef swarm.Address, publisher *ecdsa.PublicKey, tag string) (swarm.Address, error)
	UploadHandler(ref swarm.Address, publisher *ecdsa.PublicKey, topic string) (swarm.Address, error)
}

type defaultController struct {
	history        History
	granteeManager GranteeManager
	accessLogic    AccessLogic
}

func (c *defaultController) DownloadHandler(timestamp int64, enryptedRef swarm.Address, publisher *ecdsa.PublicKey, tag string) (swarm.Address, error) {
	act, err := c.history.Lookup(timestamp)
	if err != nil {
		return swarm.EmptyAddress, err
	}
	addr, err := c.accessLogic.Get(act, enryptedRef, *publisher, tag)
	return addr, err
}

func (c *defaultController) UploadHandler(ref swarm.Address, publisher *ecdsa.PublicKey, topic string) (swarm.Address, error) {
	act, _ := c.history.Lookup(0)
	if act == nil {
		// new feed
		act = NewInMemoryAct()
		act = c.granteeManager.Publish(act, *publisher, topic)
	}
	//FIXME: check if ACT is consistent with the grantee list
	return c.accessLogic.EncryptRef(act, *publisher, ref)
}

func NewController(history History, granteeManager GranteeManager, accessLogic AccessLogic) Controller {
	return &defaultController{
		history:        history,
		granteeManager: granteeManager,
		accessLogic:    accessLogic,
	}
}
