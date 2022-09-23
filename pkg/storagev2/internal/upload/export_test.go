// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package upload

import "time"

var (
	ErrTagIDAddressItemMarshalAddressIsZero = errTagIDAddressItemMarshalAddressIsZero
	ErrTagIDAddressItemUnmarshalInvalidSize = errTagIDAddressItemUnmarshalInvalidSize

	ErrPushItemMarshalAddressIsZero = errPushItemMarshalAddressIsZero
	ErrPushItemUnmarshalInvalidSize = errPushItemUnmarshalInvalidSize
)

type (
	TagIDAddressItem = tagIDAddressItem
	PushItem         = pushItem
)

func ReplaceTimeNow(fn func() time.Time) { now = fn }
