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
	"sort"

	"github.com/spf13/viper"
)

const (
	ScriptsFolderProperty     = "kubernetes.folders.scripts"
	ScriptsFolderDefaultValue = "provision/kubernetes/scripts"

	NodePortStart = 30000
	NodePortEnd   = 32767
)

type KubernetesConfiguration struct {
	ConfigurationFile        string
	RegistriesSecret         string
	UsedPorts                sort.IntSlice
	RegistriesSecrets        map[string]string
	DeploymentsConfiguration map[string]interface{}
}

func (c *KubernetesConfiguration) GetNewFreePort() int {

	// Initialize with the first available port
	if c.UsedPorts == nil || len(c.UsedPorts) == 0 {
		c.UsedPorts = sort.IntSlice{NodePortStart}
		return NodePortStart
	}

	// Only one port used
	if len(c.UsedPorts) == 1 {

		// The port used is bigger than the minimum. Use the minimum and update the port list
		if NodePortStart < c.UsedPorts[0] {
			c.UsedPorts = sort.IntSlice{NodePortStart, c.UsedPorts[0]}
			return NodePortStart
		}

		// We just have the initial port. Use the next one and update the port list
		if NodePortStart == c.UsedPorts[0] {
			c.UsedPorts = sort.IntSlice{c.UsedPorts[0], NodePortStart + 1}
			return NodePortStart + 1
		}
	}

	// More than one port. Find a gap
	for i := 0; i < len(c.UsedPorts)-1; i++ {
		diff := c.UsedPorts[i+1] - c.UsedPorts[i]
		if diff > 1 {
			port := c.UsedPorts[i] + 1
			base := append(c.UsedPorts[:i], port)
			c.UsedPorts = append(base, c.UsedPorts[i:]...)
			return port
		}
	}

	// There isn't any gap. Return the next to the last one if it's still in range
	lastPort := c.UsedPorts[len(c.UsedPorts)-1]
	if lastPort < NodePortEnd {
		c.UsedPorts = append(c.UsedPorts, lastPort+1)
		return lastPort + 1
	}

	return -1
}

func (c *KubernetesConfiguration) ClaimPort(port int) error {
	if !c.portInRange(port) {
		return fmt.Errorf("Port %d is outside the NodePort allowed range", port)
	}

	if c.UsedPorts == nil || len(c.UsedPorts) == 0 {
		c.UsedPorts = sort.IntSlice{port}
		return nil
	}

	idx := c.UsedPorts.Search(port)
	if idx == len(c.UsedPorts) {
		c.UsedPorts = append(c.UsedPorts, port)
	} else {
		if c.UsedPorts[idx] == port {
			return fmt.Errorf("Port %d is already in use", port)
		}

		base := append(c.UsedPorts[:idx], port)
		c.UsedPorts = append(base, c.UsedPorts[idx:]...)
	}

	return nil
}

func (c KubernetesConfiguration) portInRange(port int) bool {
	return port >= NodePortStart && port <= NodePortEnd
}

func (c *KubernetesConfiguration) LiberatePort(port int) {
	if c.UsedPorts != nil && len(c.UsedPorts) > 0 {
		idx := c.UsedPorts.Search(port)
		if idx < len(c.UsedPorts) && c.UsedPorts[idx] == port {
			c.UsedPorts = append(c.UsedPorts[:idx], c.UsedPorts[idx+1:]...)
		}
	}
}

type KubernetesProvisioner interface {
	Provision(config *KubernetesConfiguration, deploymentID string, infra *model.InfrastructureDeploymentInfo, args model.Parameters) error
}

type KubernetesController struct {
	ScriptsFolder       string
	productProvisioners map[string]KubernetesProvisioner
}

func NewKubernetesController() *KubernetesController {
	viper.SetDefault(ScriptsFolderProperty, ScriptsFolderDefaultValue)
	scriptsFolder := viper.GetString(ScriptsFolderProperty)
	return &KubernetesController{
		ScriptsFolder: scriptsFolder,
		productProvisioners: map[string]KubernetesProvisioner{
			"rook": RookProvisioner{
				scriptsFolder: scriptsFolder,
			},
			"mysql":    MySQLProvisioner{},
			"services": GenericServiceProvisioner{},
		},
	}
}

func GetKubernetesConfiguration(i model.InfrastructureDeploymentInfo) (KubernetesConfiguration, error) {
	var result KubernetesConfiguration
	ok, err := utils.GetStruct(i.Products, "kubernetes", &result)
	if !ok {
		err = fmt.Errorf("Can't find kubernetes configuration in infrastructure %s", i.ID)
	}
	return result, err
}

func (p *KubernetesController) AddProvisioner(name string, provisioner KubernetesProvisioner) {
	p.productProvisioners[name] = provisioner
}

func (p KubernetesController) initializeConfig(config *KubernetesConfiguration) {
	if config.DeploymentsConfiguration == nil {
		config.DeploymentsConfiguration = make(map[string]interface{})
	}

	if config.UsedPorts == nil {
		config.UsedPorts = make(sort.IntSlice, 0)
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

	if args == nil {
		args = make(model.Parameters)
	}

	provisioner, ok := p.productProvisioners[product]
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
