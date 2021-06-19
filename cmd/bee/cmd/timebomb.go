package cmd

import (
	"log"
	"strconv"
	"time"

	"github.com/ethersphere/bee"
	"github.com/ethersphere/bee/pkg/logging"
)

const (
	nostartDayCount = 90
	warningDayCount = 0.9 * nostartDayCount // show warning once 90% of the time bomb time has passed
	sleepFor        = 30 * time.Minute
)

var (
	commitTime, _   = strconv.ParseInt(bee.CommitTime, 10, 64)
	versionReleased = time.Unix(commitTime, 0)
)

func startTimeBomb(logger logging.Logger) {
	acceptedWarnDate := time.Now().AddDate(0, 0, -nostartDayCount)

	if versionReleased.Before(acceptedWarnDate) {
		log.Fatal("your bee version is outdated, stopping. Please check for the latest version")
	}

	acceptedWarnDate = time.Now().AddDate(0, 0, -warningDayCount)

	if versionReleased.Before(acceptedWarnDate) {
		logger.Warning("your node is almost outdated, please check for the latest version")
	}

	<-time.After(sleepFor)
}
