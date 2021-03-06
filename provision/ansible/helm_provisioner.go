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

type HelmProvisioner struct {
	parent *Provisioner
}

func NewHelmProvisioner(parent *Provisioner) HelmProvisioner {
	return HelmProvisioner{
		parent: parent,
	}
}

func (p HelmProvisioner) BuildInventory(infra *model.InfrastructureDeploymentInfo, args model.Parameters) (Inventory, error) {
	return DefaultKubernetesInventory(*infra), nil
}

func (p HelmProvisioner) DeployProduct(inventoryPath string, infra *model.InfrastructureDeploymentInfo, args model.Parameters) (model.Parameters, error) {

	logger := logrus.WithFields(logrus.Fields{
		"infrastructure": infra.ID,
	})
	infra.Products["helm"] = true

	return nil, ExecutePlaybook(logger, p.parent.ScriptsFolder+"/kubernetes/deploy_helm.yml", inventoryPath, nil)
}
