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
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"time"

	homedir "github.com/mitchellh/go-homedir"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

const (
	ConfigurationFolderName = "deployment-engine"
)

type knownHostsLine struct {
	marker  string
	hosts   []string
	pubKey  ssh.PublicKey
	comment string
}

type knownHostsMap map[string][]knownHostsLine

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

// WaitForStatusChange calls the getter function during the time specified in timeout or until it returns a value which is different than the one specified in the "status" parameter.
// It returns the final status, if there was a timeout and if the getter function returned error at any moment.
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

func GetStruct(input map[string]interface{}, key string, result interface{}) (bool, error) {
	raw, ok := input[key]
	if !ok {
		return false, nil
	}

	return true, TransformObject(raw, result)
}

func TransformObject(input interface{}, output interface{}) error {
	strInput, err := json.Marshal(input)
	if err != nil {
		return err
	}
	return json.Unmarshal(strInput, output)
}

func GetObjectFromMap(src map[string]interface{}, key string, result interface{}) (bool, error) {
	raw, ok := src[key]
	if !ok {
		return false, nil
	}
	return ok, TransformObject(raw, result)
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

func WrapLogAndReturnError(log *log.Entry, message string, err error) error {
	if err != nil {
		log.WithError(err).Error(message)
		return fmt.Errorf("%s: %w", message, err)
	}
	log.Error(message)
	return errors.New(message)
}

func connectSSH(host model.NodeInfo, signer ssh.Signer) (knownHostsLine, error) {
	var result knownHostsLine
	config := &ssh.ClientConfig{
		User:              host.Username,
		HostKeyAlgorithms: []string{ssh.SigAlgoRSA},
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.HostKeyCallback(func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			result.hosts = []string{remote.String()}
			result.pubKey = key
			return nil
		}),
	}

	_, timeout, _ := WaitForStatusChange("not_connected", 60*time.Second, func() (string, error) {
		_, connError := ssh.Dial("tcp", host.IP+":22", config)
		if connError != nil {
			return "not_connected", nil
		}
		return "connected", nil
	})
	if timeout {
		return result, fmt.Errorf("Timeout connecting to host %s", host.Hostname)
	}

	return result, nil
}

func addToKnownHosts(knownHostsLocation string, hosts []knownHostsLine) error {

	f, err := os.OpenFile(knownHostsLocation,
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	for _, host := range hosts {
		line := knownhosts.Line(host.hosts, host.pubKey)
		_, err = fmt.Fprintln(f, line)
		if err != nil {
			return fmt.Errorf("Error writing host %v line to known hosts: %w", host.hosts, err)
		}
	}

	return nil
}

func readKnownHosts(knownHostsLocation string) (knownHostsMap, error) {
	result := make(knownHostsMap)

	content, err := ioutil.ReadFile(knownHostsLocation)
	if err != nil {
		return result, fmt.Errorf("Error reading known hosts file %s: %w", knownHostsLocation, err)
	}

	var currentKnownHost knownHostsLine
	rest := content
	for err == nil {
		currentKnownHost.marker, currentKnownHost.hosts, currentKnownHost.pubKey, currentKnownHost.comment, rest, err = ssh.ParseKnownHosts(rest)
		for _, host := range currentKnownHost.hosts {
			lines, ok := result[host]
			if !ok {
				lines = make([]knownHostsLine, 0, 1)
			}
			lines = append(lines, currentKnownHost)
			result[host] = lines
		}
	}

	return result, nil
}

func WaitForSSHReady(infra model.InfrastructureDeploymentInfo, addToKNownHosts bool) error {
	sshFolderLocation := os.Getenv("HOME") + "/.ssh"
	privateKeyLocation := sshFolderLocation + "/id_rsa"
	knownHostsLocation := sshFolderLocation + "/known_hosts"

	key, err := ioutil.ReadFile(privateKeyLocation)
	if err != nil {
		return err
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return err
	}

	toAppend := make([]knownHostsLine, 0)
	for _, hosts := range infra.Nodes {
		for _, host := range hosts {
			line, err := connectSSH(host, signer)
			if err != nil {
				return err
			}
			toAppend = append(toAppend, line)
		}

	}

	if addToKNownHosts {
		return addToKnownHosts(knownHostsLocation, toAppend)
	}

	return nil
}
