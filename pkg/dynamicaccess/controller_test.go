package dynamicaccess_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/hex"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/dynamicaccess"
	"github.com/ethersphere/bee/pkg/dynamicaccess/mock"
	"github.com/ethersphere/bee/pkg/encryption"
	kvsmock "github.com/ethersphere/bee/pkg/kvs/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"golang.org/x/crypto/sha3"
)

var hashFunc = sha3.NewLegacyKeccak256

func mockTestHistory(key, val []byte) dynamicaccess.History {
	var (
		h   = mock.NewHistory()
		now = time.Now()
		s   = kvsmock.New()
	)
	_ = s.Put(key, val)
	h.Insert(now.AddDate(-3, 0, 0).Unix(), s)
	return h
}

func TestDecrypt(t *testing.T) {
	pk := getPrivateKey()
	ak := encryption.Key([]byte("cica"))

	si := dynamicaccess.NewDefaultSession(pk)
	aek, _ := si.Key(&pk.PublicKey, [][]byte{{0}, {1}})
	e2 := encryption.New(aek[1], 0, uint32(0), hashFunc)
	peak, _ := e2.Encrypt(ak)

	h := mockTestHistory(aek[0], peak)
	al := setupAccessLogic(pk)
	gm := dynamicaccess.NewGranteeManager(al)
	c := dynamicaccess.NewController(h, gm, al)
	eref, ref := prepareEncryptedChunkReference(ak)
	// ech := al.EncryptRef(ch, "tag")

	ts := int64(0)
	addr, err := c.DownloadHandler(ts, eref, &pk.PublicKey, "tag")
	if err != nil {
		t.Fatalf("DownloadHandler() returned an error: %v", err)
	}
	if !addr.Equal(ref) {
		t.Fatalf("Decrypted chunk address: %s is not the expected: %s", addr, ref)
	}
}

func TestEncrypt(t *testing.T) {
	pk := getPrivateKey()
	ak := encryption.Key([]byte("cica"))

	si := dynamicaccess.NewDefaultSession(pk)
	aek, _ := si.Key(&pk.PublicKey, [][]byte{{0}, {1}})
	e2 := encryption.New(aek[1], 0, uint32(0), hashFunc)
	peak, _ := e2.Encrypt(ak)

	h := mockTestHistory(aek[0], peak)
	al := setupAccessLogic(pk)
	gm := dynamicaccess.NewGranteeManager(al)
	c := dynamicaccess.NewController(h, gm, al)
	eref, ref := prepareEncryptedChunkReference(ak)

	key1, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	gm.Add("topic", []*ecdsa.PublicKey{&key1.PublicKey})

	addr, _ := c.UploadHandler(ref, &pk.PublicKey, "topic")
	if !addr.Equal(eref) {
		t.Fatalf("Decrypted chunk address: %s is not the expected: %s", addr, eref)
	}
}

func prepareEncryptedChunkReference(ak []byte) (swarm.Address, swarm.Address) {
	addr, _ := hex.DecodeString("f7b1a45b70ee91d3dbfd98a2a692387f24db7279a9c96c447409e9205cf265baef29bf6aa294264762e33f6a18318562c86383dd8bfea2cec14fae08a8039bf3")
	e1 := encryption.New(ak, 0, uint32(0), hashFunc)
	ech, err := e1.Encrypt(addr)
	if err != nil {
		return swarm.EmptyAddress, swarm.NewAddress(addr)
	}
	return swarm.NewAddress(ech), swarm.NewAddress(addr)
}

func getPrivateKey() *ecdsa.PrivateKey {
	data, _ := hex.DecodeString("c786dd84b61485de12146fd9c4c02d87e8fd95f0542765cb7fc3d2e428c0bcfa")

	privKey, _ := crypto.DecodeSecp256k1PrivateKey(data)
	return privKey
}
