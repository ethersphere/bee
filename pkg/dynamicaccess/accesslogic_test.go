package dynamicaccess_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/hex"
	"testing"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/dynamicaccess"
	kvsmock "github.com/ethersphere/bee/pkg/kvs/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

// Generates a new test environment with a fix private key
func setupAccessLogic2() dynamicaccess.ActLogic {
	privateKey := getPrivKey(1)
	diffieHellman := dynamicaccess.NewDefaultSession(privateKey)
	al := dynamicaccess.NewLogic(diffieHellman)

	return al
}

func getPrivKey(keyNumber int) *ecdsa.PrivateKey {
	var keyHex string

	switch keyNumber {
	case 0:
		keyHex = "a786dd84b61485de12146fd9c4c02d87e8fd95f0542765cb7fc3d2e428c0bcfa"
	case 1:
		keyHex = "b786dd84b61485de12146fd9c4c02d87e8fd95f0542765cb7fc3d2e428c0bcfb"
	case 2:
		keyHex = "c786dd84b61485de12146fd9c4c02d87e8fd95f0542765cb7fc3d2e428c0bcfc"
	default:
		panic("Invalid key number")
	}

	data, err := hex.DecodeString(keyHex)
	if err != nil {
		panic(err)
	}

	privKey, err := crypto.DecodeSecp256k1PrivateKey(data)
	if err != nil {
		panic(err)
	}

	return privKey
}

func TestDecryptRef_Success(t *testing.T) {
	id0 := getPrivKey(0)
	s := kvsmock.New()
	al := setupAccessLogic2()
	err := al.AddPublisher(s, &id0.PublicKey)
	if err != nil {
		t.Errorf("AddPublisher: expected no error, got %v", err)
	}

	byteRef, _ := hex.DecodeString("39a5ea87b141fe44aa609c3327ecd896c0e2122897f5f4bbacf74db1033c5559")

	expectedRef := swarm.NewAddress(byteRef)
	t.Logf("encryptedRef: %s", expectedRef.String())

	encryptedRef, err := al.EncryptRef(s, &id0.PublicKey, expectedRef)
	t.Logf("encryptedRef: %s", encryptedRef.String())
	if err != nil {
		t.Errorf("There was an error while calling EncryptRef: ")
		t.Error(err)
	}

	acutalRef, err := al.DecryptRef(s, encryptedRef, &id0.PublicKey)
	if err != nil {
		t.Errorf("There was an error while calling Get: ")
		t.Error(err)
	}

	if expectedRef.Compare(acutalRef) != 0 {

		t.Errorf("Get gave back wrong Swarm reference!")
	}
}

func TestDecryptRefWithGrantee_Success(t *testing.T) {
	id0, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	diffieHellman := dynamicaccess.NewDefaultSession(id0)
	al := dynamicaccess.NewLogic(diffieHellman)

	s := kvsmock.New()
	err := al.AddPublisher(s, &id0.PublicKey)
	if err != nil {
		t.Errorf("AddPublisher: expected no error, got %v", err)
	}

	id1, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	err = al.AddGrantee(s, &id0.PublicKey, &id1.PublicKey, nil)
	if err != nil {
		t.Errorf("AddNewGrantee: expected no error, got %v", err)
	}

	byteRef, _ := hex.DecodeString("39a5ea87b141fe44aa609c3327ecd896c0e2122897f5f4bbacf74db1033c5559")

	expectedRef := swarm.NewAddress(byteRef)
	t.Logf("encryptedRef: %s", expectedRef.String())

	encryptedRef, err := al.EncryptRef(s, &id0.PublicKey, expectedRef)
	t.Logf("encryptedRef: %s", encryptedRef.String())
	if err != nil {
		t.Errorf("There was an error while calling EncryptRef: ")
		t.Error(err)
	}

	diffieHellman2 := dynamicaccess.NewDefaultSession(id1)
	granteeAccessLogic := dynamicaccess.NewLogic(diffieHellman2)
	acutalRef, err := granteeAccessLogic.DecryptRef(s, encryptedRef, &id0.PublicKey)
	if err != nil {
		t.Errorf("There was an error while calling Get: ")
		t.Error(err)
	}

	if expectedRef.Compare(acutalRef) != 0 {

		t.Errorf("Get gave back wrong Swarm reference!")
	}
}

func TestDecryptRef_Error(t *testing.T) {
	id0 := getPrivKey(0)

	s := kvsmock.New()
	al := setupAccessLogic2()
	err := al.AddPublisher(s, &id0.PublicKey)
	if err != nil {
		t.Errorf("AddPublisher: expected no error, got %v", err)
	}

	expectedRef := "39a5ea87b141fe44aa609c3327ecd896c0e2122897f5f4bbacf74db1033c5559"

	encryptedRef, _ := al.EncryptRef(s, &id0.PublicKey, swarm.NewAddress([]byte(expectedRef)))

	r, err := al.DecryptRef(s, encryptedRef, nil)
	if err == nil {
		t.Logf("r: %s", r.String())
		t.Errorf("Get should return encrypted access key not found error!")
	}
}

func TestAddPublisher(t *testing.T) {
	id0 := getPrivKey(0)
	savedLookupKey := "b6ee086390c280eeb9824c331a4427596f0c8510d5564bc1b6168d0059a46e2b"
	s := kvsmock.New()

	al := setupAccessLogic2()
	err := al.AddPublisher(s, &id0.PublicKey)
	if err != nil {
		t.Errorf("AddPublisher: expected no error, got %v", err)
	}

	decodedSavedLookupKey, err := hex.DecodeString(savedLookupKey)
	if err != nil {
		t.Errorf("DecodeString: expected no error, got %v", err)
	}

	encryptedAccessKey, err := s.Get(decodedSavedLookupKey)
	if err != nil {
		t.Errorf("Lookup: expected no error, got %v", err)
	}
	decodedEncryptedAccessKey := hex.EncodeToString(encryptedAccessKey)

	// A random value is returned so it is only possibly to check the length of the returned value
	// We know the lookup key because the generated private key is fixed
	if len(decodedEncryptedAccessKey) != 64 {
		t.Errorf("AddPublisher: expected encrypted access key length 64, got %d", len(decodedEncryptedAccessKey))
	}
	if s == nil {
		t.Errorf("AddPublisher: expected act, got nil")
	}
}

func TestAddNewGranteeToContent(t *testing.T) {

	id0 := getPrivKey(0)
	id1 := getPrivKey(1)
	id2 := getPrivKey(2)

	publisherLookupKey := "b6ee086390c280eeb9824c331a4427596f0c8510d5564bc1b6168d0059a46e2b"
	firstAddedGranteeLookupKey := "a13678e81f9d939b9401a3ad7e548d2ceb81c50f8c76424296e83a1ad79c0df0"
	secondAddedGranteeLookupKey := "d5e9a6499ca74f5b8b958a4b89b7338045b2baa9420e115443a8050e26986564"

	s := kvsmock.New()
	al := setupAccessLogic2()
	err := al.AddPublisher(s, &id0.PublicKey)
	if err != nil {
		t.Errorf("AddNewGrantee: expected no error, got %v", err)
	}

	err = al.AddGrantee(s, &id0.PublicKey, &id1.PublicKey, nil)
	if err != nil {
		t.Errorf("AddNewGrantee: expected no error, got %v", err)
	}

	err = al.AddGrantee(s, &id0.PublicKey, &id2.PublicKey, nil)
	if err != nil {
		t.Errorf("AddNewGrantee: expected no error, got %v", err)
	}

	lookupKeyAsByte, err := hex.DecodeString(publisherLookupKey)
	if err != nil {
		t.Errorf("AddNewGrantee: expected no error, got %v", err)
	}
	result, _ := s.Get(lookupKeyAsByte)
	hexEncodedEncryptedAK := hex.EncodeToString(result)
	if len(hexEncodedEncryptedAK) != 64 {
		t.Errorf("AddNewGrantee: expected encrypted access key length 64, got %d", len(hexEncodedEncryptedAK))
	}

	lookupKeyAsByte, err = hex.DecodeString(firstAddedGranteeLookupKey)
	if err != nil {
		t.Errorf("AddNewGrantee: expected no error, got %v", err)
	}
	result, _ = s.Get(lookupKeyAsByte)
	hexEncodedEncryptedAK = hex.EncodeToString(result)
	if len(hexEncodedEncryptedAK) != 64 {
		t.Errorf("AddNewGrantee: expected encrypted access key length 64, got %d", len(hexEncodedEncryptedAK))
	}

	lookupKeyAsByte, err = hex.DecodeString(secondAddedGranteeLookupKey)
	if err != nil {
		t.Errorf("AddNewGrantee: expected no error, got %v", err)
	}
	result, _ = s.Get(lookupKeyAsByte)
	hexEncodedEncryptedAK = hex.EncodeToString(result)
	if len(hexEncodedEncryptedAK) != 64 {
		t.Errorf("AddNewGrantee: expected encrypted access key length 64, got %d", len(hexEncodedEncryptedAK))
	}
}
