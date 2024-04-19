package dynamicaccess

import (
	"context"
	"crypto/ecdsa"
	"time"

	"github.com/ethersphere/bee/v2/pkg/file/loadsave"
	"github.com/ethersphere/bee/v2/pkg/file/pipeline"
	"github.com/ethersphere/bee/v2/pkg/file/pipeline/builder"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/kvs"
	kvsmock "github.com/ethersphere/bee/v2/pkg/kvs/mock"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

type GranteeManager interface {
	//PUT /grantees/{grantee}
	//body: {publisher?, grantee root hash ,grantee}
	Grant(ctx context.Context, granteesAddress swarm.Address, grantee *ecdsa.PublicKey) error
	//DELETE /grantees/{grantee}
	//body: {publisher?, grantee root hash , grantee}
	Revoke(ctx context.Context, granteesAddress swarm.Address, grantee *ecdsa.PublicKey) error
	//[ ]
	//POST /grantees
	//body: {publisher, historyRootHash}
	Commit(ctx context.Context, granteesAddress swarm.Address, actRootHash swarm.Address, publisher *ecdsa.PublicKey) (swarm.Address, swarm.Address, error)

	//Post /grantees
	//{publisher, addList, removeList}
	HandleGrantees(ctx context.Context, rootHash swarm.Address, publisher *ecdsa.PublicKey, addList, removeList []*ecdsa.PublicKey) error

	//GET /grantees/{history root hash}
	GetGrantees(ctx context.Context, rootHash swarm.Address) ([]*ecdsa.PublicKey, error)
}

// TODO: add granteeList ref to history metadata to solve inconsistency
type Controller interface {
	GranteeManager
	// DownloadHandler decrypts the encryptedRef using the lookupkey based on the history and timestamp.
	DownloadHandler(ctx context.Context, encryptedRef swarm.Address, publisher *ecdsa.PublicKey, historyRootHash swarm.Address, timestamp int64) (swarm.Address, error)
	// TODO: history encryption
	// UploadHandler encrypts the reference and stores it in the history as the latest update.
	UploadHandler(ctx context.Context, reference swarm.Address, publisher *ecdsa.PublicKey, historyRootHash swarm.Address) (swarm.Address, swarm.Address, swarm.Address, error)
}

type controller struct {
	accessLogic ActLogic
	granteeList GranteeList
	//[ ]: do we need to protect this with a mutex?
	revokeFlag []swarm.Address
	getter     storage.Getter
	putter     storage.Putter
}

var _ Controller = (*controller)(nil)

func (c *controller) DownloadHandler(
	ctx context.Context,
	encryptedRef swarm.Address,
	publisher *ecdsa.PublicKey,
	historyRootHash swarm.Address,
	timestamp int64,
) (swarm.Address, error) {
	ls := loadsave.New(c.getter, c.putter, requestPipelineFactory(ctx, c.putter, false, redundancy.NONE))
	history, err := NewHistoryReference(ls, historyRootHash)
	if err != nil {
		return swarm.ZeroAddress, err
	}
	entry, err := history.Lookup(ctx, timestamp)
	if err != nil {
		return swarm.ZeroAddress, err
	}
	// TODO: hanlde granteelist ref in mtdt
	kvs, err := kvs.NewReference(ls, entry.Reference())
	if err != nil {
		return swarm.ZeroAddress, err
	}

	return c.accessLogic.DecryptRef(ctx, kvs, encryptedRef, publisher)
}

func (c *controller) UploadHandler(
	ctx context.Context,
	refrefence swarm.Address,
	publisher *ecdsa.PublicKey,
	historyRootHash swarm.Address,
) (swarm.Address, swarm.Address, swarm.Address, error) {
	ls := loadsave.New(c.getter, c.putter, requestPipelineFactory(ctx, c.putter, false, redundancy.NONE))
	historyRef := historyRootHash
	var (
		storage    kvs.KeyValueStore
		storageRef swarm.Address
	)
	now := time.Now().Unix()
	if historyRef.IsZero() {
		history, err := NewHistory(ls)
		if err != nil {
			return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
		}
		storage, err = kvs.New(ls)
		if err != nil {
			return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
		}
		err = c.accessLogic.AddPublisher(ctx, storage, publisher)
		if err != nil {
			return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
		}
		storageRef, err = storage.Save(ctx)
		if err != nil {
			return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
		}
		// TODO: pass granteelist ref as mtdt
		err = history.Add(ctx, storageRef, &now, nil)
		if err != nil {
			return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
		}
		historyRef, err = history.Store(ctx)
		if err != nil {
			return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
		}
	} else {
		history, err := NewHistoryReference(ls, historyRef)
		if err != nil {
			return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
		}
		// TODO: hanlde granteelist ref in mtdt
		entry, err := history.Lookup(ctx, now)
		storageRef = entry.Reference()
		if err != nil {
			return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
		}
		storage, err = kvs.NewReference(ls, storageRef)
		if err != nil {
			return swarm.ZeroAddress, swarm.ZeroAddress, swarm.ZeroAddress, err
		}
	}

	encryptedRef, err := c.accessLogic.EncryptRef(ctx, storage, publisher, refrefence)
	return storageRef, historyRef, encryptedRef, err
}

func NewController(ctx context.Context, accessLogic ActLogic, getter storage.Getter, putter storage.Putter) Controller {
	return &controller{
		granteeList: nil,
		accessLogic: accessLogic,
		getter:      getter,
		putter:      putter,
	}
}

func (c *controller) Grant(ctx context.Context, granteesAddress swarm.Address, grantee *ecdsa.PublicKey) error {
	return c.granteeList.Add([]*ecdsa.PublicKey{grantee})
}

func (c *controller) Revoke(ctx context.Context, granteesAddress swarm.Address, grantee *ecdsa.PublicKey) error {
	if !c.isRevokeFlagged(granteesAddress) {
		c.setRevokeFlag(granteesAddress, true)
	}
	return c.granteeList.Remove([]*ecdsa.PublicKey{grantee})
}

func (c *controller) Commit(ctx context.Context, granteesAddress swarm.Address, actRootHash swarm.Address, publisher *ecdsa.PublicKey) (swarm.Address, swarm.Address, error) {
	var act kvs.KeyValueStore
	if c.isRevokeFlagged(granteesAddress) {
		act = kvsmock.New()
		c.accessLogic.AddPublisher(ctx, act, publisher)
	} else {
		act = kvsmock.NewReference(actRootHash)
	}

	grantees := c.granteeList.Get()
	for _, grantee := range grantees {
		c.accessLogic.AddGrantee(ctx, act, publisher, grantee, nil)
	}

	granteeref, err := c.granteeList.Save(ctx)
	if err != nil {
		return swarm.EmptyAddress, swarm.EmptyAddress, err
	}

	actref, err := act.Save(ctx)
	if err != nil {
		return swarm.EmptyAddress, swarm.EmptyAddress, err
	}

	c.setRevokeFlag(granteesAddress, false)
	return granteeref, actref, err
}

func (c *controller) HandleGrantees(ctx context.Context, granteesAddress swarm.Address, publisher *ecdsa.PublicKey, addList, removeList []*ecdsa.PublicKey) error {
	act := kvsmock.New()

	c.accessLogic.AddPublisher(ctx, act, publisher)
	for _, grantee := range addList {
		c.accessLogic.AddGrantee(ctx, act, publisher, grantee, nil)
	}
	return nil
}

func (c *controller) GetGrantees(ctx context.Context, granteeRootHash swarm.Address) ([]*ecdsa.PublicKey, error) {
	return c.granteeList.Get(), nil
}

func (c *controller) isRevokeFlagged(granteeRootHash swarm.Address) bool {
	for _, revoke := range c.revokeFlag {
		if revoke.Equal(granteeRootHash) {
			return true
		}
	}
	return false
}

func (c *controller) setRevokeFlag(granteeRootHash swarm.Address, set bool) {
	if set {
		c.revokeFlag = append(c.revokeFlag, granteeRootHash)
	} else {
		for i, revoke := range c.revokeFlag {
			if revoke.Equal(granteeRootHash) {
				c.revokeFlag = append(c.revokeFlag[:i], c.revokeFlag[i+1:]...)
			}
		}
	}
}

func requestPipelineFactory(ctx context.Context, s storage.Putter, encrypt bool, rLevel redundancy.Level) func() pipeline.Interface {
	return func() pipeline.Interface {
		return builder.NewPipelineBuilder(ctx, s, encrypt, rLevel)
	}
}
