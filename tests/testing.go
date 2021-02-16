package tests

import (
	"log"
	"os"
)

// BaseTestPath defines where the tests can write
const BaseTestPath = "/tmp/insights-operator-tests"

func init() {
	_, err := os.Stat(BaseTestPath)

	if os.IsNotExist(err) {
		errDir := os.MkdirAll(BaseTestPath, 0755)
		if errDir != nil {
			log.Fatal(err)
		}
	}
}
