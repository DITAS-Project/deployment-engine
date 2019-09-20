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

type DockerProvisioner struct {
	parent *Provisioner
}

func NewDockerProvisioner(parent *Provisioner) DockerProvisioner {
	return DockerProvisioner{
		parent: parent,
	}
}

func (p DockerProvisioner) buildHost(host model.NodeInfo) InventoryHost {
	return InventoryHost{
		Name: host.Hostname,
		Vars: map[string]string{
			"ansible_host": host.IP,
			"ansible_user": host.Username,
		},
	}
}

func (p DockerProvisioner) BuildInventory(infra *model.InfrastructureDeploymentInfo, args model.Parameters) (Inventory, error) {

	result := Inventory{
		Hosts: make([]InventoryHost, 0),
	}

	infra.ForEachNode(func(node model.NodeInfo) {
		result.Hosts = append(result.Hosts, p.buildHost(node))
	})

	return result, nil
}

func (p DockerProvisioner) DeployProduct(inventoryPath string, infra *model.InfrastructureDeploymentInfo, args model.Parameters) error {

	logger := logrus.WithField("product", "docker")
	err := utils.ExecuteCommand(logger, "ansible-galaxy", "install", "geerlingguy.docker")
	if err != nil {
		return err
	}

	return ExecutePlaybook(logger, p.parent.ScriptsFolder+"/docker/main.yml", inventoryPath, nil)
}
