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
	RegistriesSecret         string
	LastNodePort             int
	UsedPorts                map[int]bool
	RegistriesSecrets        map[string]string
	DeploymentsConfiguration map[string]interface{}
}

func (c *KubernetesConfiguration) GetNewFreePort() int {

	if c.LastNodePort == 0 {
		c.LastNodePort = 30000
	}

	_, ok := c.UsedPorts[c.LastNodePort]
	for ok {
		c.LastNodePort++
		_, ok = c.UsedPorts[c.LastNodePort]
	}

	if c.portInRange(c.LastNodePort) {
		c.UsedPorts[c.LastNodePort] = true
		return c.LastNodePort
	}

	return -1
}

func (c *KubernetesConfiguration) ClaimPort(port int) error {
	if !c.portInRange(port) {
		return fmt.Errorf("Port %d is outside of the NodePort range", port)
	}

	_, ok := c.UsedPorts[port]
	if ok {
		return fmt.Errorf("Port %d is already in use", port)
	}

	c.UsedPorts[port] = true
	return nil
}

func (c KubernetesConfiguration) portInRange(port int) bool {
	return port >= 30000 && port <= 32767
}

func (c *KubernetesConfiguration) LiberatePort(port int) {
	_, ok := c.UsedPorts[port]
	if ok {
		delete(c.UsedPorts, port)
	}

	if port < c.LastNodePort {
		c.LastNodePort = port
	}
}

type KubernetesProvisioner interface {
	Provision(config *KubernetesConfiguration, deploymentID string, infra *model.InfrastructureDeploymentInfo, args model.Parameters) error
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
			"mysql":    MySQLProvisioner{},
			"services": GenericServiceProvisioner{},
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

	if config.UsedPorts == nil {
		config.UsedPorts = make(map[int]bool)
	}

	if config.RegistriesSecrets == nil {
		config.RegistriesSecrets = make(map[string]string)
	}
}

func (p KubernetesController) Provision(deploymentId string, infra *model.InfrastructureDeploymentInfo, product string, args model.Parameters) error {
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
