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

package ansible

import (
	"deployment-engine/model"

	"github.com/sirupsen/logrus"
)

type KubesprayProvisioner struct {
	parent          *Provisioner
	kubesprayFolder string
}

func NewKubesprayProvisioner(parent *Provisioner, kubesprayFolder string) KubesprayProvisioner {
	return KubesprayProvisioner{
		parent:          parent,
		kubesprayFolder: kubesprayFolder,
	}
}

func (p KubesprayProvisioner) BuildInventory(infra *model.InfrastructureDeploymentInfo, args model.Parameters) (Inventory, error) {
	baseInventory, err := p.parent.Provisioners["kubernetes"].BuildInventory(infra, args)
	if err != nil {
		return baseInventory, err
	}

	master, err := infra.GetFirstNodeOfRole("master")
	if err != nil {
		return baseInventory, err
	}
	masterGroup := InventoryGroup{
		Name:  "kube-master",
		Hosts: []string{master.Hostname},
	}

	slavesGroup := InventoryGroup{
		Name:  "kube-node",
		Hosts: make([]string, len(baseInventory.Hosts)),
	}

	etcdGroup := InventoryGroup{
		Name:  "etcd",
		Hosts: []string{master.Hostname},
	}

	childrenGroup := InventoryGroup{
		Name:  "k8s-cluster:children",
		Hosts: []string{"kube-master", "kube-node"},
	}

	for i, host := range baseInventory.Hosts {
		slavesGroup.Hosts[i] = host.Name
	}

	baseInventory.Groups = []InventoryGroup{masterGroup, slavesGroup, etcdGroup, childrenGroup}
	return baseInventory, err
}

func (p KubesprayProvisioner) DeployProduct(inventoryPath string, infra *model.InfrastructureDeploymentInfo, args model.Parameters) (model.Parameters, error) {

	logger := logrus.WithFields(logrus.Fields{
		"infrastructure": infra.ID,
	})
	return nil, ExecutePlaybook(logger, p.kubesprayFolder+"/cluster.yml", inventoryPath, nil)
}
