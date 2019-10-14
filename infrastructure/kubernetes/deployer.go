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

package kubernetes

import (
	"deployment-engine/model"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	"deployment-engine/utils"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

const availablePortRangeProperty = "available_ports_range"

type KubernetesDeployer struct {
	deploymentsFolder string
}

func NewKubernetesDeployer(deploymentsFolder string) *KubernetesDeployer {
	return &KubernetesDeployer{
		deploymentsFolder: deploymentsFolder,
	}
}

func (d KubernetesDeployer) transformNode(node model.ResourceType) model.NodeInfo {
	result := model.NodeInfo{
		Hostname:   node.Name,
		Role:       node.Role,
		CPU:        node.CPU,
		Cores:      node.Cores,
		RAM:        node.RAM,
		IP:         node.IP,
		DriveSize:  node.Disk,
		DataDrives: make([]model.DriveInfo, len(node.Drives)),
	}

	for i := range node.Drives {
		result.DataDrives[i] = model.DriveInfo{
			Name: node.Drives[i].Name,
			Size: node.Drives[i].Size,
		}
	}

	return result
}

func (d KubernetesDeployer) transformNodes(nodes []model.ResourceType) map[string][]model.NodeInfo {
	result := make(map[string][]model.NodeInfo)
	for _, node := range nodes {
		roleNodes, ok := result[node.Role]
		if !ok {
			roleNodes = make([]model.NodeInfo, 0, 1)
		}
		roleNodes = append(roleNodes, d.transformNode(node))
		result[node.Role] = roleNodes
	}
	return result
}

func (d KubernetesDeployer) DeployInfrastructure(infra model.InfrastructureType) (model.InfrastructureDeploymentInfo, error) {
	deployment := model.InfrastructureDeploymentInfo{
		ID:              uuid.New().String(),
		Name:            infra.Name,
		Products:        make(map[string]interface{}),
		ExtraProperties: infra.ExtraProperties,
		Nodes:           make(map[string][]model.NodeInfo),
	}

	logger := log.WithField("infrastructure", deployment.ID)

	infraFolder := fmt.Sprintf("%s/%s", d.deploymentsFolder, deployment.ID)
	err := os.Mkdir(infraFolder, os.ModePerm)
	if err != nil {
		return deployment, utils.WrapLogAndReturnError(logger, fmt.Sprintf("Error creating infrastructure folder %s", infraFolder), err)
	}
	configFile, ok := infra.Provider.Credentials["config"]
	if !ok {
		return deployment, errors.New("Configuration file in credentials.config is mandatory for pre-existing kubernetes clusters")
	}
	strConfig, err := yaml.Marshal(configFile)
	if err != nil {
		return deployment, utils.WrapLogAndReturnError(logger, "Error marshaling kubernetes configuration file", err)
	}

	configPath := fmt.Sprintf("%s/%s", infraFolder, "config")
	err = ioutil.WriteFile(configPath, strConfig, 0644)
	if err != nil {
		return deployment, utils.WrapLogAndReturnError(logger, "Error writing kubernetes configuration file", err)
	}

	kubeConfig := map[string]interface{}{
		"configurationfile": configPath,
	}

	registriesSecretRaw, ok := infra.Provider.Credentials["registries_secret"]
	if ok {
		registriesSecret, ok := registriesSecretRaw.(string)
		if !ok {
			return deployment, utils.WrapLogAndReturnError(logger, "Error getting kubernetes registries secret value. It must be a string", err)
		}
		kubeConfig["registriessecret"] = registriesSecret
	}

	kubeConfig["managed"] = false

	portRangeIn, ok := infra.ExtraProperties[availablePortRangeProperty]
	if ok {
		portRange := strings.Split(portRangeIn, "-")
		if portRange == nil || len(portRange) != 2 {
			return deployment, fmt.Errorf("Port range must be in the format 'portStart-portEnd' but found %s", portRangeIn)
		}

		minPort, err := strconv.Atoi(portRange[0])
		if err != nil {
			return deployment, fmt.Errorf("Invalid port range start value %s: %w", portRange[0], err)
		}

		maxPort, err := strconv.Atoi(portRange[1])
		if err != nil {
			return deployment, fmt.Errorf("Invalid port range end value %s: %w", portRange[1], err)
		}

		kubeConfig["portrange"] = struct {
			PortStart int
			PortEnd   int
		}{
			minPort,
			maxPort,
		}
	}

	deployment.Products["kubernetes"] = kubeConfig

	deployment.Nodes = d.transformNodes(infra.Resources)

	return deployment, nil
}

func (d KubernetesDeployer) DeleteInfrastructure(infra model.InfrastructureDeploymentInfo) map[string]error {
	return nil
}
