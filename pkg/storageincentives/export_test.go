// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storageincentives

var (
	NewEvents             = newEvents
	SaveSample            = saveSample
	GetSample             = getSample
	RemoveSample          = removeSample
	SaveCommitKey         = saveCommitKey
	GetCommitKey          = getCommitKey
	RemoveCommitKey       = removeCommitKey
	SaveRevealRound       = saveRevealRound
	GetRevealRound        = getRevealRound
	RemoveRevealRound     = removeRevealRound
	SaveLastPurgedRound   = saveLastPurgedRound
	GetLastPurgedRound    = getLastPurgedRound
	PurgeStaleDataHandler = purgeStaleDataHandler
)

type (
	SampleData = sampleData
)

const (
	PurgeStaleDataThreshold = purgeStaleDataThreshold
)
