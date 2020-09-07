package blocklist

import "time"

func SetTimeNow(f func() time.Time) {
	timeNow = f
}
