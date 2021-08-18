// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"crypto/rand"
	"encoding/base64"
	"time"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"golang.org/x/crypto/bcrypt"
)

type authRecord struct {
	role   string
	expiry time.Time
}

type apiKeys map[string]authRecord

type Authenticator struct {
	username     string
	passwordHash []byte
	keys         apiKeys
	expires      time.Duration
	enforcer     *casbin.Enforcer
}

func New(username, passwordHash string, expires time.Duration) (*Authenticator, error) {
	m, err := model.NewModelFromString(`
	[request_definition]
	r = sub, obj, act
	
	[policy_definition]
	p = sub, obj, act
	
	[policy_effect]
	e = some(where (p.eft == allow))
	
	[matchers]
	m = r.sub == p.sub && keyMatch(r.obj, p.obj) && regexMatch(r.act, p.act)`)

	if err != nil {
		return nil, err
	}

	e, err := casbin.NewEnforcer(m)
	if err != nil {
		return nil, err
	}

	if err := applyPolicies(e); err != nil {
		return nil, err
	}

	auth := Authenticator{
		username:     username,
		passwordHash: []byte(passwordHash),
		keys:         make(apiKeys),
		expires:      expires,
		enforcer:     e,
	}

	return &auth, nil
}

func (a *Authenticator) Authorize(username, password string) bool {
	if a.username != username {
		return false
	}
	return nil == bcrypt.CompareHashAndPassword(a.passwordHash, []byte(password))
}

func (a *Authenticator) AddKey(role string) (string, error) {
	now := time.Now()

	ar := authRecord{
		role:   role,
		expiry: now.Add(a.expires),
	}

	b := make([]byte, 16)

	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}

	apiKey := base64.URLEncoding.EncodeToString(b)

	a.keys[apiKey] = ar

	return apiKey, nil
}

func (a *Authenticator) Enforce(apiKey, obj, act string) (bool, error) {
	authRecord, found := a.keys[apiKey]
	if !found {
		return false, nil
	}

	if time.Now().After(authRecord.expiry) {
		delete(a.keys, apiKey)
		return false, nil
	}

	allow, err := a.enforcer.Enforce(authRecord.role, obj, act)
	if err != nil {
		return false, err
	}

	return allow, nil
}

func applyPolicies(e *casbin.Enforcer) error {
	_, err := e.AddPolicies([][]string{
		{"role0", "/bytes/*", "GET"},
		{"role1", "/bytes", "POST"},
		{"role0", "/chunks/*", "GET"},
		{"role1", "/chunks", "POST"},
		{"role0", "/bzz/*", "GET"},
		{"role1", "/bzz/*", "PATCH"},
		{"role1", "/bzz", "POST"},
		{"role0", "/bzz/*/*", "GET"},
		{"role1", "/tags", "(GET)|(POST)"},
		{"role1", "/tags/*", "(GET)|(DELETE)|(PATCH)"},
		{"role1", "/pins/*", "(GET)|(DELETE)|(POST)"},
		{"role2", "/pins", "GET"},
		{"role1", "/pss/send/*", "POST"},
		{"role0", "/pss/subscribe/*", "GET"},
		{"role1", "/soc/*/*", "POST"},
		{"role1", "/feeds/*/*", "POST"},
		{"role0", "/feeds/*/*", "GET"},
		{"role2", "/stamps", "GET"},
		{"role2", "/stamps/*", "GET"},
		{"role2", "/stamps/*/*", "POST"},
		{"role2", "/addresses", "GET"},
		{"role2", "/blocklist", "GET"},
		{"role2", "/connect/*", "POST"},
		{"role2", "/peers", "GET"},
		{"role2", "/peers/*", "DELETE"},
		{"role2", "/pingpong/*", "POST"},
		{"role2", "/topology", "GET"},
		{"role2", "/welcome-message", "(GET)|(POST)"},
		{"role2", "/balances", "GET"},
		{"role2", "/balances/*", "GET"},
		{"role2", "/chequebook/cashout/*", "GET"},
		{"role3", "/chequebook/cashout/*", "POST"},
		{"role3", "/chequebook/withdraw", "POST"},
		{"role3", "/chequebook/deposit", "POST"},
		{"role2", "/chequebook/cheque/*", "GET"},
		{"role2", "/chequebook/cheque", "GET"},
		{"role2", "/chequebook/address", "GET"},
		{"role2", "/chequebook/balance", "GET"},
		{"role2", "/chunks/*", "(GET)|(DELETE)"},
		{"role2", "/reservestate", "GET"},
		{"role2", "/chainstate", "GET"},
		{"role2", "/settlements/*", "GET"},
		{"role2", "/settlements", "GET"},
		{"role2", "/transactions", "GET"},
		{"role0", "/transactions/*", "GET"},
		{"role3", "/transactions/*", "(POST)|(DELETE)"},
		{"role0", "/readiness", "GET"},
		{"role0", "/health", "GET"},
		{"role0", "/consumed", "GET"},
		{"role0", "/consumed/*", "GET"},
		{"role0", "/chunks/stream", "GET"},
		{"role0", "/stewardship/*", "PUT"},
	})

	return err
}
