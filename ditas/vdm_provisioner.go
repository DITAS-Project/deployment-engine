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

type VDMProvisioner struct {
	parent              *ansible.Provisioner
	scriptsFolder       string
	configVariablesPath string
	configFolder        string
}

func NewVDMProvisioner(parent *ansible.Provisioner, scriptsFolder, configVariablesPath, configFolder string) VDMProvisioner {
	return VDMProvisioner{
		parent:              parent,
		scriptsFolder:       scriptsFolder,
		configVariablesPath: configVariablesPath,
		configFolder:        configFolder,
	}
}

func (p VDMProvisioner) BuildInventory(deploymentID string, infra model.InfrastructureDeploymentInfo, args map[string][]string) (ansible.Inventory, error) {
	return p.parent.Provisioners["kubeadm"].BuildInventory(deploymentID, infra, args)
}

func (p VDMProvisioner) DeployProduct(inventoryPath, deploymentID string, infra model.InfrastructureDeploymentInfo, args map[string][]string) error {

	logger := logrus.WithFields(logrus.Fields{
		"deployment":     deploymentID,
		"infrastructure": infra.ID,
	})

	return ansible.ExecutePlaybook(logger, p.scriptsFolder+"/deploy_vdm.yml", inventoryPath, map[string]string{
		"vars_file":     p.configVariablesPath,
		"config_folder": p.configFolder,
	})
}
