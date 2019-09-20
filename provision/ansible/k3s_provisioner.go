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

	"strconv"

	"github.com/sirupsen/logrus"
)

const (
	K3sCurlInstalled = "k3s_curl_installed"
)

type K3sProvisioner struct {
	parent *Provisioner
}

func NewK3sProvisioner(parent *Provisioner) K3sProvisioner {
	return K3sProvisioner{
		parent: parent,
	}
}

func (p K3sProvisioner) BuildInventory(infra *model.InfrastructureDeploymentInfo, args model.Parameters) (Inventory, error) {
	return p.parent.Provisioners["kubeadm"].BuildInventory(infra, args)
}

func (p K3sProvisioner) DeployProduct(inventoryPath string, infra *model.InfrastructureDeploymentInfo, args model.Parameters) error {

	logger := logrus.WithFields(logrus.Fields{
		"infrastructure": infra.ID,
	})

	inventoryFolder := p.parent.GetInventoryFolder(infra.ID)

	master, err := infra.GetFirstNodeOfRole("master")
	if err != nil {
		return err
	}

	err = ExecutePlaybook(logger, p.parent.ScriptsFolder+"/kubernetes/deploy_k3s.yml", inventoryPath, map[string]string{
		"master_ip":        master.IP,
		"inventory_folder": inventoryFolder,
		"install_curl":     strconv.FormatBool(!infra.ExtraProperties.GetBool(K3sCurlInstalled)),
	})
	if err != nil {
		logger.WithError(err).Error("Error initializing master")
		return err
	}

	infra.Products["kubernetes"] = KubernetesConfiguration{
		ConfigurationFile: inventoryFolder + "/config",
	}

	return err
}
