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

package provision

import (
	"deployment-engine/model"
	"deployment-engine/persistence"
	"deployment-engine/utils"
	"fmt"

	log "github.com/sirupsen/logrus"
)

type ProvisionerController struct {
	Repository  persistence.DeploymentRepository
	Provisioner model.Provisioner
}

func (p *ProvisionerController) Provision(deploymentID, infraID, product string) (model.DeploymentInfo, error) {
	deployment, err := p.Repository.Get(deploymentID)
	if err != nil {
		log.WithError(err).Errorf("Error getting deployment %s", deploymentID)
		return deployment, err
	}

	i, infra, err := utils.FindInfra(deployment, infraID)
	if err != nil {
		log.WithError(err).Errorf("Error finding infrastructure %s", infraID)
		return deployment, err
	}

	for _, prod := range infra.Products {
		if prod == product {
			return deployment, fmt.Errorf("Product %s already present in the infrastructure", product)
		}
	}

	err = p.Provisioner.Provision(deploymentID, *infra, product)
	if err != nil {
		log.WithError(err).Errorf("Error provisioning product %s", product)
		return deployment, err
	}

	deployment.Infrastructures[i].Products = append(deployment.Infrastructures[i].Products, product)
	deployment, err = p.Repository.Update(deployment)
	if err != nil {
		log.WithError(err).Errorf("Error updating deployment information")
		return deployment, err
	}

	return deployment, err
}
