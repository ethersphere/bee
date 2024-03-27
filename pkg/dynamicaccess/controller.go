package dynamicaccess

import (
	"crypto/ecdsa"

	kvsmock "github.com/ethersphere/bee/pkg/kvs/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

type Controller interface {
	DownloadHandler(timestamp int64, enryptedRef swarm.Address, publisher *ecdsa.PublicKey, tag string) (swarm.Address, error)
	UploadHandler(ref swarm.Address, publisher *ecdsa.PublicKey, topic string) (swarm.Address, error)
}

type defaultController struct {
	history        History
	granteeManager GranteeManager
	accessLogic    ActLogic
}

func (c *defaultController) DownloadHandler(timestamp int64, enryptedRef swarm.Address, publisher *ecdsa.PublicKey, tag string) (swarm.Address, error) {
	kvs, err := c.history.Lookup(timestamp)
	if err != nil {
		return swarm.EmptyAddress, err
	}
	addr, err := c.accessLogic.DecryptRef(kvs, enryptedRef, publisher)
	return addr, err
}

func (c *defaultController) UploadHandler(ref swarm.Address, publisher *ecdsa.PublicKey, topic string) (swarm.Address, error) {
	kvs, err := c.history.Lookup(0)
	if err != nil {
		return swarm.EmptyAddress, err
	}
	if kvs == nil {
		// new feed
		// TODO: putter session to create kvs
		kvs = kvsmock.New()
		_, err = c.granteeManager.Publish(kvs, publisher, topic)
		if err != nil {
			return swarm.EmptyAddress, err
		}
	}
	//FIXME: check if kvs is consistent with the grantee list
	return c.accessLogic.EncryptRef(kvs, publisher, ref)
}

func NewController(history History, granteeManager GranteeManager, accessLogic ActLogic) Controller {
	return &defaultController{
		history:        history,
		granteeManager: granteeManager,
		accessLogic:    accessLogic,
	}
}
