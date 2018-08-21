package utils

import (
	"os/exec"
	"time"

	log "github.com/sirupsen/logrus"
)

func ExecuteCommand(logger *log.Entry, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = logger.Writer()
	cmd.Stderr = logger.Writer()
	return cmd.Run()
}

func WaitForStatusChange(status string, timeout time.Duration, getter func() (string, error)) (string, bool, error) {
	waited := 0 * time.Second
	currentStatus := status
	var err error
	for currentStatus, err = getter(); currentStatus == status && waited < timeout && err == nil; currentStatus, err = getter() {
		time.Sleep(3 * time.Second)
		waited += 3 * time.Second
		//fmt.Print(".")
	}
	return currentStatus, waited >= timeout, err
}
