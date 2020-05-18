// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pullsync

/*

Test cases:
get range 0,5:
- no chunks (interval should be then sealed)
- interval full (all chunks present)
- some chunks available in the middle
- 1 chunk available at the corners of the interval (1 and 5)
- 1 chunk available at the corners and one in the middle

get range 0,10:
- no chunks
- some chunks but still reach 0,10 (4 chunks across the entire interval)
- chunk at 1,10, 3,5/4,9
- 1 chunk beginning | 1 chunk end
- 1 beginning & 1 end

- reach max request page size
- - 1-5 full
- - 1-7 (holes in the middle)
- - 2-6 (holes in the middle)
- - 2-9 (holes in the middle)
*/
