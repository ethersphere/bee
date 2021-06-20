package cmd

import (
	"strconv"
	"time"

	"github.com/ethersphere/bee"
	"github.com/ethersphere/bee/pkg/logging"
)

const (
	limitDays   = 90
	warningDays = 0.9 * limitDays // show warning once 90% of the time bomb time has passed
	sleepFor    = 30 * time.Minute
)

var (
	commitTime, _   = strconv.ParseInt(bee.CommitTime, 10, 64)
	versionReleased = time.Unix(commitTime, 0)
)

func startTimeBomb(logger logging.Logger) {
	for {
		outdated := time.Now().AddDate(0, 0, -limitDays)

		if versionReleased.Before(outdated) {
			logger.Warning("your node is outdated, please check for the latest version")
		} else {
			almostOutdated := time.Now().AddDate(0, 0, -warningDays)

			if versionReleased.Before(almostOutdated) {
				logger.Warning("your node is almost outdated, please check for the latest version")
			}
		}

		<-time.After(sleepFor)
	}
}

func endSupportDate() string {
	return versionReleased.AddDate(0, 0, limitDays).Format("2 January 2006")
}
