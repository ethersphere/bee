// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package getter

import "errors"

func IsCannotRecoverError(err error, missingChunks int) bool {
	return errors.Is(err, cannotRecoverError(missingChunks))
}

func IsNotRecoveredError(err error, chAddress string) bool {
	return errors.Is(err, notRecoveredError(chAddress))
}

func IsNoDataAddressIncludedError(err error, chAddress string) bool {
	return errors.Is(err, noDataAddressIncludedError(chAddress))
}

func IsNoRedundancyError(err error, chAddress string) bool {
	return errors.Is(err, noRedundancyError(chAddress))
}
