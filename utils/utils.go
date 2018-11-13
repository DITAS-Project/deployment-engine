/**
 * Copyright 2018 Atos
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not
 * use this file except in compliance with the License. You may obtain a copy of
 * the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
 * WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
 * License for the specific language governing permissions and limitations under
 * the License.
 */

package utils

import (
	"deployment-engine/model"
	"fmt"
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

func FindInfra(deployment model.DeploymentInfo, infraID string) (int, *model.InfrastructureDeploymentInfo, error) {
	for i, infra := range deployment.Infrastructures {
		if infra.ID == infraID {
			return i, &infra, nil
		}
	}

	return 0, nil, fmt.Errorf("Infrastructure %s not found in deployment %s", infraID, deployment.ID)
}
