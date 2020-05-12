// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testing

import (
	"testing"

	"github.com/ethersphere/bee/pkg/swarm"
	mockbytes "gitlab.com/nolash/go-mockbytes"
)

var (
	fileByteMod int = 255
	fileLengths     = []int{
		31,                           // 0
		32,                           // 1
		33,                           // 2
		63,                           // 3
		64,                           // 4
		65,                           // 5
		swarm.ChunkSize,              // 6
		swarm.ChunkSize + 31,         // 7
		swarm.ChunkSize + 32,         // 8
		swarm.ChunkSize + 63,         // 9
		swarm.ChunkSize + 64,         // 10
		swarm.ChunkSize * 2,          // 11
		swarm.ChunkSize*2 + 32,       // 12
		swarm.ChunkSize * 128,        // 13
		swarm.ChunkSize*128 + 31,     // 14
		swarm.ChunkSize*128 + 32,     // 15
		swarm.ChunkSize*128 + 64,     // 16
		swarm.ChunkSize * 129,        // 17
		swarm.ChunkSize * 130,        // 18
		swarm.ChunkSize * 128 * 128,  // 19
		swarm.ChunkSize*128*128 + 32, // 20
	}
	fileExpectHashHex = []string{
		"ece86edb20669cc60d142789d464d57bdf5e33cb789d443f608cbd81cfa5697d", // 0
		"0be77f0bb7abc9cd0abed640ee29849a3072ccfd1020019fe03658c38f087e02", // 1
		"3463b46d4f9d5bfcbf9a23224d635e51896c1daef7d225b86679db17c5fd868e", // 2
		"95510c2ff18276ed94be2160aed4e69c9116573b6f69faaeed1b426fea6a3db8", // 3
		"490072cc55b8ad381335ff882ac51303cc069cbcb8d8d3f7aa152d9c617829fe", // 4
		"541552bae05e9a63a6cb561f69edf36ffe073e441667dbf7a0e9a3864bb744ea", // 5
		"c10090961e7682a10890c334d759a28426647141213abda93b096b892824d2ef", // 6
		"91699c83ed93a1f87e326a29ccd8cc775323f9e7260035a5f014c975c5f3cd28", // 7
		"73759673a52c1f1707cbb61337645f4fcbd209cdc53d7e2cedaaa9f44df61285", // 8
		"db1313a727ffc184ae52a70012fbbf7235f551b9f2d2da04bf476abe42a3cb42", // 9
		"ade7af36ac0c7297dc1c11fd7b46981b629c6077bce75300f85b02a6153f161b", // 10
		"29a5fb121ce96194ba8b7b823a1f9c6af87e1791f824940a53b5a7efe3f790d9", // 11
		"61416726988f77b874435bdd89a419edc3861111884fd60e8adf54e2f299efd6", // 12
		"3047d841077898c26bbe6be652a2ec590a5d9bd7cd45d290ea42511b48753c09", // 13
		"e5c76afa931e33ac94bce2e754b1bb6407d07f738f67856783d93934ca8fc576", // 14
		"485a526fc74c8a344c43a4545a5987d17af9ab401c0ef1ef63aefcc5c2c086df", // 15
		"624b2abb7aefc0978f891b2a56b665513480e5dc195b4a66cd8def074a6d2e94", // 16
		"b8e1804e37a064d28d161ab5f256cc482b1423d5cd0a6b30fde7b0f51ece9199", // 17
		"59de730bf6c67a941f3b2ffa2f920acfaa1713695ad5deea12b4a121e5f23fa1", // 18
		"522194562123473dcfd7a457b18ee7dee8b7db70ed3cfa2b73f348a992fdfd3b", // 19
		"ed0cc44c93b14fef2d91ab3a3674eeb6352a42ac2f0bbe524711824aae1e7bcc", // 20
	}
)

// GetVector returns test data corresponding to the test vector index,
// and the expected result address.
func GetVector(t *testing.T, idx int) ([]byte, swarm.Address) {
	t.Helper()
	if idx > fileLengths[idx] {
		t.Fatalf("idx %d out of bound for count %d", idx, GetVectorCount())
	}
	g := mockbytes.New(0, mockbytes.MockTypeStandard).WithModulus(fileByteMod)
	data, err := g.SequentialBytes(fileLengths[idx])
	if err != nil {
		t.Fatal(err)
	}
	return data, swarm.MustParseHexAddress(fileExpectHashHex[idx])
}

// GetVectorCount returns the number of available test vectors.
func GetVectorCount() int {
	return len(fileLengths)
}
