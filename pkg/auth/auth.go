// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"

	"github.com/casbin/casbin/v2"
	"github.com/ethersphere/bee/pkg/logging"
	"golang.org/x/crypto/bcrypt"
)

type authRecord struct {
	Role   string    `json:"role"`
	Expiry time.Time `json:"expiry"`
}

type Authenticator struct {
	passwordHash []byte
	ciph         *encrypter
	enforcer     *casbin.Enforcer
	log          logging.Logger
}

func New(encryptionKey, passwordHash string, logger logging.Logger) (*Authenticator, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	e, err := casbin.NewEnforcer(dir+"/bee/security.conf", dir+"/bee/policy.csv")
	if err != nil {
		return nil, err
	}

	ciph, err := newEncrypter([]byte(encryptionKey))
	if err != nil {
		return nil, err
	}

	auth := Authenticator{
		enforcer:     e,
		ciph:         ciph,
		passwordHash: []byte(passwordHash),
		log:          logger,
	}

	return &auth, nil
}

func (a *Authenticator) Authorize(password string) bool {
	return nil == bcrypt.CompareHashAndPassword(a.passwordHash, []byte(password))
}

func (a *Authenticator) GenerateKey(role string, expires int) (string, error) {
	ar := authRecord{
		Role:   role,
		Expiry: time.Now().Add(time.Second * time.Duration(expires)),
	}

	data, err := json.Marshal(ar)
	if err != nil {
		return "", err
	}

	encryptedBytes, err := a.ciph.encrypt(data)
	if err != nil {
		return "", err
	}

	apiKey := base64.StdEncoding.EncodeToString(encryptedBytes)

	return apiKey, nil
}

var ErrTokenExpired = errors.New("token expired")

func (a *Authenticator) RefreshKey(apiKey string, expiry int) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(apiKey)
	if err != nil {
		return "", err
	}

	decryptedBytes, err := a.ciph.decrypt(decoded)
	if err != nil {
		return "", err
	}

	var ar authRecord
	if err := json.Unmarshal(decryptedBytes, &ar); err != nil {
		return "", err
	}

	if time.Now().After(ar.Expiry) {
		return "", ErrTokenExpired
	}

	ar.Expiry = time.Now().Add(time.Duration(expiry) * time.Second)

	data, err := json.Marshal(ar)
	if err != nil {
		return "", err
	}

	encryptedBytes, err := a.ciph.encrypt(data)
	if err != nil {
		return "", err
	}

	apiKey = base64.StdEncoding.EncodeToString(encryptedBytes)

	return apiKey, nil
}

func (a *Authenticator) Enforce(apiKey, obj, act string) (bool, error) {
	decoded, err := base64.StdEncoding.DecodeString(apiKey)
	if err != nil {
		a.log.Error("decode token", err)
		return false, err
	}

	decryptedBytes, err := a.ciph.decrypt(decoded)
	if err != nil {
		a.log.Error("decrypt token", err)
		return false, err
	}

	var ar authRecord
	if err := json.Unmarshal(decryptedBytes, &ar); err != nil {
		a.log.Error("unmarshal token", err)
		return false, err
	}

	if time.Now().After(ar.Expiry) {
		a.log.Error("token expired")
		return false, ErrTokenExpired
	}

	allow, err := a.enforcer.Enforce(ar.Role, obj, act)
	if err != nil {
		a.log.Error("enforce", err)
		return false, err
	}

	return allow, nil
}

type encrypter struct {
	gcm cipher.AEAD
}

func newEncrypter(key []byte) (*encrypter, error) {
	hasher := md5.New()
	_, err := hasher.Write(key)
	if err != nil {
		return nil, err
	}
	hash := hex.EncodeToString(hasher.Sum(nil))
	block, err := aes.NewCipher([]byte(hash))
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	return &encrypter{
		gcm: gcm,
	}, nil
}

func (e encrypter) encrypt(data []byte) ([]byte, error) {
	nonce := make([]byte, e.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	ciphertext := e.gcm.Seal(nonce, nonce, data, nil)
	return ciphertext, nil
}

func (e encrypter) decrypt(data []byte) ([]byte, error) {
	nonceSize := e.gcm.NonceSize()
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := e.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}
	return plaintext, nil
}
