// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package upload

import "time"

var (
	ErrPushItemMarshalAddressIsZero = errPushItemMarshalAddressIsZero
	ErrPushItemMarshalBatchInvalid  = errPushItemMarshalBatchInvalid
	ErrPushItemUnmarshalInvalidSize = errPushItemUnmarshalInvalidSize

	ErrTagItemUnmarshalInvalidSize = errTagItemUnmarshalInvalidSize

	ErrUploadItemMarshalAddressIsZero = errUploadItemMarshalAddressIsZero
	ErrUploadItemMarshalBatchInvalid  = errUploadItemMarshalBatchInvalid
	ErrUploadItemUnmarshalInvalidSize = errUploadItemUnmarshalInvalidSize

	ErrPutterAlreadyClosed       = errPutterAlreadyClosed
	ErrOverwriteOfImmutableBatch = errOverwriteOfImmutableBatch
	ErrOverwriteOfNewerBatch     = errOverwriteOfNewerBatch

	ErrNextTagIDUnmarshalInvalidSize = errNextTagIDUnmarshalInvalidSize

	ErrDirtyTagItemUnmarshalInvalidSize = errDirtyTagItemUnmarshalInvalidSize
)

type (
	PushItem     = pushItem
	UploadItem   = uploadItem
	NextTagID    = nextTagID
	DirtyTagItem = dirtyTagItem
)

func ReplaceTimeNow(fn func() time.Time) { now = fn }
