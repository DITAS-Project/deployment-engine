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

type K3sProvisioner struct {
	parent        *ansible.Provisioner
	scriptsFolder string
}

func NewK3sProvisioner(parent *ansible.Provisioner, scriptsFolder string) K3sProvisioner {
	return K3sProvisioner{
		parent:        parent,
		scriptsFolder: scriptsFolder,
	}
}

func (p K3sProvisioner) BuildInventory(deploymentID string, infra model.InfrastructureDeploymentInfo) (ansible.Inventory, error) {
	return p.parent.Provisioners["kubernetes"].BuildInventory(deploymentID, infra)
}

func (p K3sProvisioner) DeployProduct(inventoryPath, deploymentID string, infra model.InfrastructureDeploymentInfo) error {

	logger := logrus.WithFields(logrus.Fields{
		"deployment":     deploymentID,
		"infrastructure": infra.ID,
	})
	err := ansible.ExecutePlaybook(logger, p.scriptsFolder+"/deploy_k3s.yml", inventoryPath, nil)
	if err != nil {
		logger.WithError(err).Error("Error initializing master")
		return err
	}

	return ansible.ExecutePlaybook(logger, p.scriptsFolder+"/deploy_k3s_nodes.yml", inventoryPath, map[string]string{
		"master_ip": infra.Master.IP,
	})
}
