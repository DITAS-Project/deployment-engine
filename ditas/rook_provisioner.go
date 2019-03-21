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
	"strconv"

	"github.com/sirupsen/logrus"
)

const (
	DitasGitInstalledProperty = "ditas_git_installed"
)

type RookProvisioner struct {
	parent        *ansible.Provisioner
	scriptsFolder string
}

func NewRookProvisioner(parent *ansible.Provisioner, scriptsFolder string) RookProvisioner {
	return RookProvisioner{
		parent:        parent,
		scriptsFolder: scriptsFolder,
	}
}

func (p RookProvisioner) BuildInventory(deploymentID string, infra model.InfrastructureDeploymentInfo) (ansible.Inventory, error) {
	return p.parent.Provisioners["kubeadm"].BuildInventory(deploymentID, infra)
}

func (p RookProvisioner) DeployProduct(inventoryPath, deploymentID string, infra model.InfrastructureDeploymentInfo) error {

	logger := logrus.WithFields(logrus.Fields{
		"deployment":     deploymentID,
		"infrastructure": infra.ID,
	})

	installGit := !infra.ExtraProperties.GetBool(DitasGitInstalledProperty)
	haAvailable := len(infra.Slaves) > 1
	numMons := 1
	if haAvailable {
		numMons = 3
	}

	return ansible.ExecutePlaybook(logger, p.scriptsFolder+"/deploy_rook.yml", inventoryPath, map[string]string{
		"ha_available": string(strconv.AppendBool([]byte{}, haAvailable)),
		"install_git":  string(strconv.AppendBool([]byte{}, installGit)),
		"num_mons":     strconv.Itoa(numMons),
	})
}
