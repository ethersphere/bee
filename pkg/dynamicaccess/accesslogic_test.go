package dynamicaccess_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/ethersphere/bee/pkg/dynamicaccess"
	"github.com/ethersphere/bee/pkg/swarm"
)

// Generates a new test environment with a fix private key
func setupAccessLogic2() dynamicaccess.ActLogic {
	privateKey := generateFixPrivateKey(1000)
	diffieHellman := dynamicaccess.NewDefaultSession(&privateKey)
	al := dynamicaccess.NewLogic(diffieHellman)

	return al
}

// Generates a fixed identity with private and public key. The private key is generated from the input
func generateFixPrivateKey(input int64) ecdsa.PrivateKey {
	fixedD := big.NewInt(input)
	curve := elliptic.P256()
	x, y := curve.ScalarBaseMult(fixedD.Bytes())

	privateKey := ecdsa.PrivateKey{
		PublicKey: ecdsa.PublicKey{
			Curve: curve,
			X:     x,
			Y:     y,
		},
		D: fixedD,
	}

	return privateKey
}

func TestGet_Success(t *testing.T) {
	al := setupAccessLogic2()
	id0 := generateFixPrivateKey(0)

	act := dynamicaccess.NewInMemoryAct()
	act, err := al.AddPublisher(act, &id0.PublicKey)
	if err != nil {
		t.Errorf("AddPublisher: expected no error, got %v", err)
	}

	byteRef, _ := hex.DecodeString("39a5ea87b141fe44aa609c3327ecd896c0e2122897f5f4bbacf74db1033c5559")

	expectedRef := swarm.NewAddress(byteRef)
	t.Logf("encryptedRef: %s", expectedRef.String())

	encryptedRef, err := al.EncryptRef(act, &id0.PublicKey, expectedRef)
	t.Logf("encryptedRef: %s", encryptedRef.String())
	if err != nil {
		t.Errorf("There was an error while calling EncryptRef: ")
		t.Error(err)
	}

	ref, err := al.Get(act, encryptedRef, &id0.PublicKey)
	if err != nil {
		t.Errorf("There was an error while calling Get: ")
		t.Error(err)
	}

	if expectedRef.Compare(ref) != 0  {

		t.Errorf("Get gave back wrong Swarm reference!")
	}
}

// This test function tests those cases where different parameters are missing
func TestGet_Error(t *testing.T) {
	al := setupAccessLogic2()
	id0 := generateFixPrivateKey(0)

	act := dynamicaccess.NewInMemoryAct()
	act, err := al.AddPublisher(act, &id0.PublicKey)
	if err != nil {
		t.Errorf("AddPublisher: expected no error, got %v", err)
	}

	expectedRef := "39a5ea87b141fe44aa609c3327ecd896c0e2122897f5f4bbacf74db1033c5559"

	encryptedRef, _ := al.EncryptRef(act, &id0.PublicKey, swarm.NewAddress([]byte(expectedRef)))

	_, err = al.Get(dynamicaccess.NewInMemoryAct(), encryptedRef, &id0.PublicKey)
	if err == nil {
		t.Errorf("Get should give back encrypted access key not found error!")
	}

	refTwo, _ := al.Get(act, swarm.EmptyAddress, &id0.PublicKey)
	if swarm.EmptyAddress.Compare(refTwo) != 0 {
		t.Errorf("Get should give back empty string if encrypted ref not provided!")
	}

	_, err = al.Get(act, encryptedRef, nil)
	if err == nil {
		t.Errorf("Get should give back error if grantee not provided!")
	}
}

func TestAddPublisher(t *testing.T) {
	al := setupAccessLogic2()
	id0 := generateFixPrivateKey(0)
	savedLookupKey := "bc36789e7a1e281436464229828f817d6612f7b477d66591ff96a9e064bcc98a"
	act := dynamicaccess.NewInMemoryAct()
	act, err := al.AddPublisher(act, &id0.PublicKey)
	if err != nil {
		t.Errorf("AddPublisher: expected no error, got %v", err)
	}

	decodedSavedLookupKey, err := hex.DecodeString(savedLookupKey)
	if err != nil {
		t.Errorf("AddPublisher: expected no error, got %v", err)
	}

	encryptedAccessKey, _ := act.Lookup(decodedSavedLookupKey)
	decodedEncryptedAccessKey := hex.EncodeToString(encryptedAccessKey)

	// A random value is returned so it is only possibly to check the length of the returned value
	// We know the lookup key because the generated private key is fixed
	if len(decodedEncryptedAccessKey) != 64 {
		t.Errorf("AddPublisher: expected encrypted access key length 64, got %d", len(decodedEncryptedAccessKey))
	}
	if act == nil {
		t.Errorf("AddPublisher: expected act, got nil")
	}
}

func TestAdd_New_Grantee_To_Content(t *testing.T) {
	al := setupAccessLogic2()

	id0 := generateFixPrivateKey(0)
	id1 := generateFixPrivateKey(1)
	id2 := generateFixPrivateKey(2)

	publisherLookupKey := "bc36789e7a1e281436464229828f817d6612f7b477d66591ff96a9e064bcc98a"
	firstAddedGranteeLookupKey := "e221a2abf64357260e8f2c937ee938aed98dce097e537c1a3fd4caf73510dbe4"
	secondAddedGranteeLookupKey := "8fe8dff7cd15a6a0095c1b25071a5691e7c901fd0b95857a96c0e4659b48716a"

	act := dynamicaccess.NewInMemoryAct()
	act, err := al.AddPublisher(act, &id0.PublicKey)
	 if err != nil {
		t.Errorf("AddNewGrantee: expected no error, got %v", err)
	}

	act, err = al.AddNewGranteeToContent(act, &id0.PublicKey, &id1.PublicKey)
	if err != nil {
		t.Errorf("AddNewGrantee: expected no error, got %v", err)
	}

	act, err = al.AddNewGranteeToContent(act, &id0.PublicKey, &id2.PublicKey)
	if err != nil {
		t.Errorf("AddNewGrantee: expected no error, got %v", err)
	}

	lookupKeyAsByte, err := hex.DecodeString(publisherLookupKey)
	if err != nil {
		t.Errorf("AddNewGrantee: expected no error, got %v", err)
	}
	result, _ := act.Lookup(lookupKeyAsByte)
	hexEncodedEncryptedAK := hex.EncodeToString(result)
	if len(hexEncodedEncryptedAK) != 64 {
		t.Errorf("AddNewGrantee: expected encrypted access key length 64, got %d", len(hexEncodedEncryptedAK))
	}

	lookupKeyAsByte, err = hex.DecodeString(firstAddedGranteeLookupKey)
	if err != nil {
		t.Errorf("AddNewGrantee: expected no error, got %v", err)
	}
	result, _ = act.Lookup(lookupKeyAsByte)
	hexEncodedEncryptedAK = hex.EncodeToString(result)
	if len(hexEncodedEncryptedAK) != 64 {
		t.Errorf("AddNewGrantee: expected encrypted access key length 64, got %d", len(hexEncodedEncryptedAK))
	}

	lookupKeyAsByte, err = hex.DecodeString(secondAddedGranteeLookupKey)
	if err != nil {
		t.Errorf("AddNewGrantee: expected no error, got %v", err)
	}
	result, _ = act.Lookup(lookupKeyAsByte)
	hexEncodedEncryptedAK = hex.EncodeToString(result)
	if len(hexEncodedEncryptedAK) != 64 {
		t.Errorf("AddNewGrantee: expected encrypted access key length 64, got %d", len(hexEncodedEncryptedAK))
	}
}

func TestEncryptRef(t *testing.T) {
	ref := "39a5ea87b141fe44aa609c3327ecd896c0e2122897f5f4bbacf74db1033c5559"
	savedEncryptedRef := "230cdcfb2e67adddb2822b38f70105213ab3e4f97d03560bfbfbb218f487c5303e9aa9a97e62aa1a8003f162679e7c65e1c8e3aacaec2043fd5d2a4a7d69285e"

	al := setupAccessLogic2()
	id0 := generateFixPrivateKey(0)
	act := dynamicaccess.NewInMemoryAct()
	decodedLookupKey, err := hex.DecodeString("bc36789e7a1e281436464229828f817d6612f7b477d66591ff96a9e064bcc98a")
	if err != nil {
		t.Errorf("EncryptRef: expected no error, got %v", err)
	}

	act.Add(decodedLookupKey, []byte("42"))
	
	encryptedRefValue, err := al.EncryptRef(act, &id0.PublicKey, swarm.NewAddress([]byte(ref)))
	if err != nil {
		t.Errorf("EncryptRef: expected no error, got %v", err)
	}

	if encryptedRefValue.String() != savedEncryptedRef {
		t.Errorf("EncryptRef: expected encrypted ref, got empty address")
	}
}