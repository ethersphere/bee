package dynamicaccess_test

// import (
// 	"crypto/ecdsa"
// 	"crypto/elliptic"
// 	"crypto/rand"
// 	"errors"
// 	"fmt"
// 	"testing"

// 	"github.com/ethersphere/bee/pkg/crypto"
// 	"github.com/ethersphere/bee/pkg/swarm"
// )

// func setupAccessLogic() AccessLogic {
// 	privateKey, err := crypto.GenerateSecp256k1Key()
// 	if err != nil {
// 		errors.New("error creating private key")
// 	}
// 	diffieHellman := NewDiffieHellman(privateKey)
// 	al := NewAccessLogic(diffieHellman)

// 	return al
// }

// func TestGetLookupKey_Success(t *testing.T) {
// 	al := setupAccessLogic()

// 	id0, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
// 	// ! this will be random, we can not know the lookup key for a randomly generated key
// 	act, encryptedRef, _ := al.ActInit(swarm.NewAddress([]byte("42")), id0.PublicKey, "")
// 	fmt.Println(act, encryptedRef)

// 	tag := "exampleTag"

// 	lookupKey, err := al.getLookUpKey(id0.PublicKey, tag)
// 	if err != nil {
// 		t.Errorf("Could not fetch lookup key from publisher and tag")
// 	}

// 	expectedLookupKey := "expectedLookupKey"
// 	if lookupKey != expectedLookupKey {
// 		fmt.Println(string(lookupKey))
// 		t.Errorf("The lookup key that was returned is not correct!")
// 	}
// }

// func TestGetLookUpKey_Error(t *testing.T) {
// 	al := setupAccessLogic()

// 	invalidPublisher := ecdsa.PublicKey{}
// 	tag := "exampleTag"

// 	lookupKey, err := al.getLookUpKey(invalidPublisher, tag)

// 	if err != nil {
// 		t.Errorf("There was an error while fetching lookup key")
// 	}

// 	if lookupKey != "" {
// 		t.Errorf("Expected lookup key to be empty for invalid input")
// 	}
// }

// func TestGetAccessKeyDecriptionKey_Success(t *testing.T) {
// 	al := setupAccessLogic()

// 	id0, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
// 	tag := "exampleTag"

// 	access_key_decryption_key, err := al.getAccessKeyDecriptionKey(id0.PublicKey, tag)
// 	if err != nil {
// 		t.Errorf("GetAccessKeyDecriptionKey gave back error")
// 	}

// 	expectedResult := "we-dont-know"
// 	if access_key_decryption_key != expectedResult {
// 		t.Errorf("The access key decryption key is not correct!")
// 	}
// }

// func TestGetAccessKeyDecriptionKey_Error(t *testing.T) {
// 	al := setupAccessLogic()

// 	invalidPublisher := ecdsa.PublicKey{}
// 	tag := "exampleTag"

// 	access_key_decryption_key, err := al.getAccessKeyDecriptionKey(invalidPublisher, tag)
// 	if err != nil {
// 		t.Errorf("GetAccessKeyDecriptionKey gave back error")
// 	}

// 	if access_key_decryption_key != "" {
// 		t.Errorf("GetAccessKeyDecriptionKey should give back empty string for invalid input!")
// 	}
// }

// func TestGetEncryptedAccessKey_Success(t *testing.T) {
// 	al := setupAccessLogic()

// 	lookupKey := "exampleLookupKey"
// 	id0, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

// 	act, _, _ := al.ActInit(swarm.NewAddress([]byte("42")), id0.PublicKey, "")

// 	encrypted_access_key, err := al.getEncryptedAccessKey(*act, lookupKey)
// 	if err != nil {
// 		t.Errorf("There was an error while executing GetEncryptedAccessKey")
// 	}

// 	expectedEncryptedKey := "abc013encryptedkey"
// 	if encrypted_access_key.Reference().String() != expectedEncryptedKey {
// 		t.Errorf("GetEncryptedAccessKey didn't give back the expected value!")
// 	}
// }

// func TestGetEncryptedAccessKey_Error(t *testing.T) {
// 	al := setupAccessLogic()

// 	lookupKey := "exampleLookupKey"
// 	id0, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

// 	act, _, _ := al.ActInit(swarm.NewAddress([]byte("42")), id0.PublicKey, "")
// 	empty_act_result, _ := al.getEncryptedAccessKey(*act, lookupKey)
// 	if empty_act_result != nil {
// 		t.Errorf("GetEncryptedAccessKey should give back nil for empty act root hash!")
// 	}

// 	empty_lookup_result, _ := al.getEncryptedAccessKey(*act, "")

// 	if empty_lookup_result != nil {
// 		t.Errorf("GetEncryptedAccessKey should give back nil for empty lookup key!")
// 	}
// }

// func TestGet_Success(t *testing.T) {
// 	al := setupAccessLogic()

// 	id0, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
// 	act, encryptedRef, _ := al.ActInit(swarm.NewAddress([]byte("42")), id0.PublicKey, "")
// 	tag := "exampleTag"

// 	ref, err := al.Get(act, encryptedRef, id0.PublicKey, tag)
// 	if err != nil {
// 		t.Errorf("There was an error while calling Get")
// 	}

// 	expectedRef := "bzzNotEncrypted128long"
// 	if ref != expectedRef {
// 		t.Errorf("Get gave back wrong Swarm reference!")
// 	}
// }

// func TestGet_Error(t *testing.T) {
// 	al := setupAccessLogic()

// 	//actRootHash := "0xabcexample"
// 	id0, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
// 	act, encrypredRef, err := al.ActInit(swarm.NewAddress([]byte("42")), id0.PublicKey, "")
// 	if err != nil {
// 		t.Errorf("Error initializing Act")
// 		t.Errorf(err.Error())
// 	}
// 	//encryptedRef := "bzzabcasab"
// 	tag := "exampleTag"

// 	refOne, err := al.Get(act, encrypredRef, id0.PublicKey, tag)
// 	if err != nil {
// 		t.Errorf(err.Error())
// 	}
// 	if refOne != "" {
// 		t.Errorf("Get should give back empty string if ACT root hash not provided!")
// 	}

// 	refTwo, _ := al.Get(act, swarm.EmptyAddress, id0.PublicKey, tag)
// 	if refTwo != "" {
// 		t.Errorf("Get should give back empty string if encrypted ref not provided!")
// 	}

// 	refThree, _ := al.Get(act, encrypredRef, ecdsa.PublicKey{}, tag)
// 	if refThree != "" {
// 		t.Errorf("Get should give back empty string if publisher not provided!")
// 	}

// 	refFour, _ := al.Get(act, encrypredRef, id0.PublicKey, "")
// 	if refFour != "" {
// 		t.Errorf("Get should give back empty string if tag was not provided!")
// 	}
// }

// func TestNewAccessLogic(t *testing.T) {
// 	logic := setupAccessLogic()

// 	_, ok := logic.(*DefaultAccessLogic)
// 	if !ok {
// 		t.Errorf("NewAccessLogic: expected type *DefaultAccessLogic, got %T", logic)
// 	}
// }

// // func TestAddGrantee(t *testing.T) {
// // 	al := setupAccessLogic()
// // 	//	ref := "example_ref"
// // 	id0, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
// // 	testGranteeList := NewGrantee()

// // 	// Add grantee keys to the testGranteeList
// // 	id1, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
// // 	id2, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
// // 	id3, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
// // 	testGranteeList.AddGrantees([]ecdsa.PublicKey{id1.PublicKey, id2.PublicKey, id3.PublicKey})

// // 	// Initialize empty ACT
// // 	actMock := MockAct.NewActMock()
// // 	actMockRootHash := "exampleRootHash"

// // 	// Add each grantee to content using ActMock and validate the resulting ACT
// // 	for i := 0; i < len(testGranteeList.GetGrantees()); i++ {
// // 		lookupKey, _ := al.getLookUpKey(testGranteeList.GetGrantees()[i], "")
// // 		encryptedAccessKey := "exampleEncryptedAccessKey"
// // 		_, err := actMock.Add(actMockRootHash, []byte(lookupKey), []byte(encryptedAccessKey))
// // 		if err != nil {
// // 			t.Fatalf("Failed to add grantee to content using ActMock: %v", err)
// // 		}

// // 		// Validate the resulting ACT
// // 		encryptedAccessKeyFromMock, err := actMock.Get(actMockRootHash, []byte(lookupKey))
// // 		if err != nil {
// // 			t.Fatalf("Failed to retrieve encrypted access key from ActMock: %v", err)
// // 		}
// // 		encryptedAccessKeyFromMockBytes, _ := hex.DecodeString(encryptedAccessKeyFromMock)
// // 		if string(encryptedAccessKeyFromMockBytes) != encryptedAccessKey {
// // 			t.Errorf("Encrypted access key retrieved from ActMock doesn't match expected value")
// // 		}
// // 	}

// // 	al.Add_New_Grantee_To_Content(actMock, encryptedRef, id0.PublicKey, testGranteeList.GetGrantees()[i])
// // }
