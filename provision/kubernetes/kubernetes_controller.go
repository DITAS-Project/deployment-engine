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
	"deployment-engine/utils"
	"errors"
	"fmt"

	"github.com/spf13/viper"
)

const (
	ScriptsFolderProperty     = "kubernetes.folders.scripts"
	ScriptsFolderDefaultValue = "provision/kubernetes/scripts"
)

type KubernetesConfiguration struct {
	ConfigurationFile        string
	LastNodePort             int
	FreedNodePorts           []int
	DeploymentsConfiguration map[string]interface{}
}

func (c *KubernetesConfiguration) GetNewFreePort() int {
	port := 0
	if c.LastNodePort == 0 {
		c.LastNodePort = 30000
	}

	if c.FreedNodePorts == nil || len(c.FreedNodePorts) == 0 {
		port = c.LastNodePort
		c.LastNodePort++
	} else {
		port = c.FreedNodePorts[len(c.FreedNodePorts)-1]
		c.FreedNodePorts = c.FreedNodePorts[:len(c.FreedNodePorts)-1]
	}

	return port
}

func (c KubernetesConfiguration) portInRange(port int) bool {
	return port >= 30000 && port <= 32767
}

func (c *KubernetesConfiguration) LiberatePort(port int) {
	if c.portInRange(port) {
		if c.FreedNodePorts == nil {
			c.FreedNodePorts = make([]int, 0)
		}
		c.FreedNodePorts = append(c.FreedNodePorts, port)
	}
}

type KubernetesProvisioner interface {
	Provision(config *KubernetesConfiguration, deploymentID string, infra *model.InfrastructureDeploymentInfo, args map[string][]string) error
}

type KubernetesController struct {
	ScriptsFolder       string
	ProductProvisioners map[string]KubernetesProvisioner
}

func NewKubernetesController() *KubernetesController {
	viper.SetDefault(ScriptsFolderProperty, ScriptsFolderDefaultValue)
	scriptsFolder := viper.GetString(ScriptsFolderProperty)
	return &KubernetesController{
		ScriptsFolder: scriptsFolder,
		ProductProvisioners: map[string]KubernetesProvisioner{
			"rook": RookProvisioner{
				scriptsFolder: scriptsFolder,
			},
			"mysql": MySQLProvisioner{},
		},
	}
}

func (p KubernetesController) initializeConfig(config *KubernetesConfiguration) {
	if config.DeploymentsConfiguration == nil {
		config.DeploymentsConfiguration = make(map[string]interface{})
	}

	if config.LastNodePort == 0 {
		config.LastNodePort = 30000
	}

	if config.FreedNodePorts == nil {
		config.FreedNodePorts = make([]int, 0)
	}
}

func (p KubernetesController) Provision(deploymentId string, infra *model.InfrastructureDeploymentInfo, product string, args map[string][]string) error {
	rawKubeConfig, ok := infra.Products["kubernetes"]
	if !ok {
		return fmt.Errorf("Kubernetes is not installed in infrastructure %s of deployment %s", infra.ID, deploymentId)
	}

	provisioner, ok := p.ProductProvisioners[product]
	if !ok {
		return fmt.Errorf("Can't find kubernetes provisioner for product %s", product)
	}

	var kubeConfig KubernetesConfiguration
	err := utils.TransformObject(rawKubeConfig, &kubeConfig)
	if err != nil {
		return fmt.Errorf("Error reading kubernetes configuration: %s", err.Error())
	}

	if kubeConfig.ConfigurationFile == "" {
		return errors.New("Can't find the configuration file in the Kubernetes configuration")
	}

	p.initializeConfig(&kubeConfig)

	err = provisioner.Provision(&kubeConfig, deploymentId, infra, args)
	if err != nil {
		return err
	}

	infra.Products["kubernetes"] = kubeConfig
	return nil
}
