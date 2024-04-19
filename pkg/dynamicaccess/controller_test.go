package dynamicaccess_test

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/dynamicaccess"
	"github.com/ethersphere/bee/v2/pkg/encryption"
	"github.com/ethersphere/bee/v2/pkg/file"
	"github.com/ethersphere/bee/v2/pkg/kvs"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/sha3"
)

var hashFunc = sha3.NewLegacyKeccak256

func getHistoryFixture(ctx context.Context, ls file.LoadSaver, al dynamicaccess.ActLogic, publisher *ecdsa.PublicKey) (swarm.Address, error) {
	h, err := dynamicaccess.NewHistory(ls, nil)
	if err != nil {
		return swarm.ZeroAddress, nil
	}
	pk1 := getPrivKey(1)
	pk2 := getPrivKey(2)

	kvs0 := kvs.New(ls, swarm.ZeroAddress)
	al.AddPublisher(ctx, kvs0, publisher)
	kvs0Ref, _ := kvs0.Save(ctx)
	kvs1 := kvs.New(ls, swarm.ZeroAddress)
	al.AddGrantee(ctx, kvs1, publisher, &pk1.PublicKey, nil)
	al.AddPublisher(ctx, kvs1, publisher)
	kvs1Ref, _ := kvs1.Save(ctx)
	kvs2 := kvs.New(ls, swarm.ZeroAddress)
	al.AddGrantee(ctx, kvs2, publisher, &pk2.PublicKey, nil)
	al.AddPublisher(ctx, kvs2, publisher)
	kvs2Ref, _ := kvs2.Save(ctx)
	firstTime := time.Date(1994, time.April, 1, 0, 0, 0, 0, time.UTC).Unix()
	secondTime := time.Date(2000, time.April, 1, 0, 0, 0, 0, time.UTC).Unix()
	thirdTime := time.Date(2015, time.April, 1, 0, 0, 0, 0, time.UTC).Unix()

	h.Add(ctx, kvs0Ref, &thirdTime)
	h.Add(ctx, kvs1Ref, &firstTime)
	h.Add(ctx, kvs2Ref, &secondTime)
	return h.Store(ctx)
}

// TODO: separate up down test with fixture, now these just check if the flow works at all
func TestController_NewUploadDownload(t *testing.T) {
	ctx := context.Background()
	publisher := getPrivKey(1)
	diffieHellman := dynamicaccess.NewDefaultSession(publisher)
	al := dynamicaccess.NewLogic(diffieHellman)
	c := dynamicaccess.NewController(ctx, al, mockStorer.ChunkStore(), mockStorer.Cache())
	ref := swarm.RandAddress(t)
	_, hRef, encryptedRef, err := c.UploadHandler(ctx, ref, &publisher.PublicKey, nil)
	assert.NoError(t, err)
	dref, err := c.DownloadHandler(ctx, time.Now().Unix(), encryptedRef, &publisher.PublicKey, hRef)
	assert.NoError(t, err)
	assert.Equal(t, ref, dref)
}

func TestController_ExistingUploadDownload(t *testing.T) {
	ls := createLs()
	ctx := context.Background()
	publisher := getPrivKey(0)
	diffieHellman := dynamicaccess.NewDefaultSession(publisher)
	al := dynamicaccess.NewLogic(diffieHellman)
	c := dynamicaccess.NewController(ctx, al, mockStorer.ChunkStore(), mockStorer.Cache())
	ref := swarm.RandAddress(t)
	hRef, err := getHistoryFixture(ctx, ls, al, &publisher.PublicKey)
	assert.NoError(t, err)
	_, hRef, encryptedRef, err := c.UploadHandler(ctx, ref, &publisher.PublicKey, &hRef)
	assert.NoError(t, err)
	dref, err := c.DownloadHandler(ctx, time.Now().Unix(), encryptedRef, &publisher.PublicKey, hRef)
	assert.NoError(t, err)
	assert.Equal(t, ref, dref)
}

func TestControllerGrant(t *testing.T) {
}

func TestControllerRevoke(t *testing.T) {

}

func TestControllerCommit(t *testing.T) {

}

func prepareEncryptedChunkReference(ak []byte) (swarm.Address, swarm.Address) {
	addr, _ := hex.DecodeString("f7b1a45b70ee91d3dbfd98a2a692387f24db7279a9c96c447409e9205cf265baef29bf6aa294264762e33f6a18318562c86383dd8bfea2cec14fae08a8039bf3")
	e1 := encryption.New(ak, 0, uint32(0), hashFunc)
	ech, err := e1.Encrypt(addr)
	if err != nil {
		return swarm.EmptyAddress, swarm.EmptyAddress
	}
	return swarm.NewAddress(ech), swarm.NewAddress(addr)
}
