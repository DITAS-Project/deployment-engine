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
 *
 * This is being developed for the DITAS Project: https://www.ditas-project.eu/
 */

package ditas

import (
	"deployment-engine/model"
	"deployment-engine/provision/ansible"

	"github.com/sirupsen/logrus"
)

const (
	KubeadmPreinstalledProperty = "ditas_preinstalled_image"
)

type KubeadmProvisioner struct {
	parent        *ansible.Provisioner
	scriptsFolder string
}

func NewKubeadmProvisioner(parent *ansible.Provisioner, scriptsFolder string) KubeadmProvisioner {
	return KubeadmProvisioner{
		parent:        parent,
		scriptsFolder: scriptsFolder,
	}
}

func (p KubeadmProvisioner) BuildInventory(deploymentID string, infra model.InfrastructureDeploymentInfo, args map[string][]string) (ansible.Inventory, error) {
	inventory, err := p.parent.Provisioners["kubernetes"].BuildInventory(deploymentID, infra, args)
	if err != nil {
		return inventory, err
	}

	slavesGroup := make([]string, len(infra.Slaves))
	for _, host := range infra.Slaves {
		slavesGroup = append(slavesGroup, host.Hostname)
	}

	inventory.Groups = []ansible.InventoryGroup{
		ansible.InventoryGroup{
			Name:  "master",
			Hosts: []string{infra.Master.Hostname},
		},
		ansible.InventoryGroup{
			Name:  "slaves",
			Hosts: slavesGroup,
		},
	}

	return inventory, err
}

func (p KubeadmProvisioner) DeployProduct(inventoryPath, deploymentID string, infra model.InfrastructureDeploymentInfo, args map[string][]string) error {

	logger := logrus.WithFields(map[string]interface{}{
		"deployment":     deploymentID,
		"infrastructure": infra.ID,
	})

	inventoryFolder := p.parent.GetInventoryFolder(deploymentID, infra.ID)

	if infra.ExtraProperties.GetBool(KubeadmPreinstalledProperty) {
		return ansible.ExecutePlaybook(logger, p.scriptsFolder+"/kubeadm.yml", inventoryPath, map[string]string{
			"inventory_folder": inventoryFolder,
		})
	}
	return p.parent.Provisioners["kubernetes"].DeployProduct(inventoryPath, deploymentID, infra, args)
}
