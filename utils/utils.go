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
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"

	homedir "github.com/mitchellh/go-homedir"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

const (
	ConfigurationFolderName = "deployment-engine"
)

func ExecuteCommand(logger *log.Entry, name string, args ...string) error {
	return CreateCommand(logger, nil, true, name, args...).Run()
}

func CreateCommand(logger *log.Entry, envVars map[string]string, preserveEnv bool, command string, args ...string) *exec.Cmd {
	cmd := exec.Command(command, args...)
	if logger != nil {
		cmd.Stdout = logger.Writer()
		cmd.Stderr = logger.Writer()
	}

	if envVars != nil {
		if preserveEnv {
			cmd.Env = os.Environ()
		} else {
			cmd.Env = make([]string, 0, len(envVars))
		}
		for k, v := range envVars {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	return cmd
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

func ConfigurationFolder() (string, error) {
	home, err := homedir.Dir()
	if err != nil {
		log.WithError(err).Error("Error getting home folder")
		return "", err
	}

	return fmt.Sprintf("%s/%s", home, ConfigurationFolderName), nil
}

func TransformObject(input interface{}, output interface{}) error {
	strInput, err := json.Marshal(input)
	if err != nil {
		return err
	}
	return json.Unmarshal(strInput, output)
}

func GetSingleValue(values map[string][]string, key string) (string, bool) {
	vals, ok := values[key]
	if !ok || vals == nil || len(vals) == 0 {
		return "", false
	}
	return vals[0], ok
}

func IndexOf(slice []int, elem int) int {
	for i, num := range slice {
		if num == elem {
			return i
		}
	}
	return -1
}

// GetDockerRepositories returns a map of Docker repositories from the configuration
func GetDockerRepositories() map[string]model.DockerRegistry {
	registries := make([]model.DockerRegistry, 0)
	result := make(map[string]model.DockerRegistry)
	viper.UnmarshalKey("kubernetes.registries", &registries)
	for _, registry := range registries {
		result[registry.Name] = registry
	}
	return result
}
