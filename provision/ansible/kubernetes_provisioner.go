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

package ansible

import (
	"deployment-engine/model"
	"deployment-engine/utils"

	"github.com/sirupsen/logrus"
)

const (
	DockerPresentProperty = "ansible_docker_installed"
)

type KubernetesProvisioner struct {
	parent *Provisioner
}

type KubernetesConfiguration struct {
	ConfigurationFile string
	RegistriesSecret  string
}

func NewKubernetesProvisioner(parent *Provisioner) *KubernetesProvisioner {
	return &KubernetesProvisioner{
		parent: parent,
	}
}

func (p KubernetesProvisioner) buildHost(host model.NodeInfo) InventoryHost {
	var role string
	if host.Role == "master" {
		role = "master"
	} else {
		role = "node"
	}

	return InventoryHost{
		Name: host.Hostname,
		Vars: map[string]string{
			"ansible_host":    host.IP,
			"ansible_user":    host.Username,
			"kubernetes_role": role,
		},
	}
}

func (p KubernetesProvisioner) BuildInventory(deploymentID string, infra *model.InfrastructureDeploymentInfo, args model.Parameters) (Inventory, error) {
	result := Inventory{
		Hosts: make([]InventoryHost, 0),
	}

	infra.ForEachNode(func(node model.NodeInfo) {
		result.Hosts = append(result.Hosts, p.buildHost(node))
	})

	return result, nil
}

func (p KubernetesProvisioner) DeployProduct(inventoryPath, deploymentID string, infra *model.InfrastructureDeploymentInfo, args model.Parameters) error {

	if !infra.ExtraProperties.GetBool(DockerPresentProperty) {
		args["wait"] = []string{"false"}
		err := p.parent.Provision(deploymentID, infra, "docker", args)
		if err != nil {
			return err
		}
	}

	logger := logrus.WithField("product", "kubernetes")
	err := utils.ExecuteCommand(logger, "ansible-galaxy", "install", "geerlingguy.kubernetes")
	if err != nil {
		return err
	}

	inventoryFolder := p.parent.GetInventoryFolder(deploymentID, infra.ID)

	err = ExecutePlaybook(logger, p.parent.ScriptsFolder+"/kubernetes/main.yml", inventoryPath, map[string]string{
		"inventory_folder": inventoryFolder,
	})
	if err != nil {
		return err
	}

	infra.Products["kubernetes"] = KubernetesConfiguration{
		ConfigurationFile: inventoryFolder + "/config",
	}

	return nil
}
