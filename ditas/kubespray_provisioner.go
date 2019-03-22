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

type KubesprayProvisioner struct {
	parent          *ansible.Provisioner
	kubesprayFolder string
}

func NewKubesprayProvisioner(parent *ansible.Provisioner, kubesprayFolder string) KubesprayProvisioner {
	return KubesprayProvisioner{
		parent:          parent,
		kubesprayFolder: kubesprayFolder,
	}
}

func (p KubesprayProvisioner) BuildInventory(deploymentID string, infra model.InfrastructureDeploymentInfo, args map[string][]string) (ansible.Inventory, error) {
	baseInventory, err := p.parent.Provisioners["kubernetes"].BuildInventory(deploymentID, infra, args)
	if err != nil {
		return baseInventory, err
	}

	masterGroup := ansible.InventoryGroup{
		Name:  "kube-master",
		Hosts: []string{infra.Master.Hostname},
	}

	slavesGroup := ansible.InventoryGroup{
		Name:  "kube-node",
		Hosts: make([]string, len(baseInventory.Hosts)),
	}

	etcdGroup := ansible.InventoryGroup{
		Name:  "etcd",
		Hosts: []string{infra.Master.Hostname},
	}

	childrenGroup := ansible.InventoryGroup{
		Name:  "k8s-cluster:children",
		Hosts: []string{"kube-master", "kube-node"},
	}

	for i, host := range baseInventory.Hosts {
		slavesGroup.Hosts[i] = host.Name
	}

	baseInventory.Groups = []ansible.InventoryGroup{masterGroup, slavesGroup, etcdGroup, childrenGroup}
	return baseInventory, err
}

func (p KubesprayProvisioner) DeployProduct(inventoryPath, deploymentID string, infra model.InfrastructureDeploymentInfo, args map[string][]string) error {

	logger := logrus.WithFields(logrus.Fields{
		"deployment":     deploymentID,
		"infrastructure": infra.ID,
	})
	return ansible.ExecutePlaybook(logger, p.kubesprayFolder+"/cluster.yml", inventoryPath, nil)
}
