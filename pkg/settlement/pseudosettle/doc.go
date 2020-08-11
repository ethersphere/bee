// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
Package pseudosettle implements a pretend settlement protocol where nodes send pretend payment messages consisting only of the payment amount.
Its purpose is to be able to have the full payment / disconnect treshold machinery in place without having to send cheques or real values.
*/
package pseudosettle
