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
	"deployment-engine/provision/kubernetes"
	"fmt"

	log "github.com/sirupsen/logrus"
)

const (
	baremetalProvisionerType = "baremetal"
)

type ProvisionerController struct {
	Repository   persistence.DeploymentRepository
	Provisioners map[string]model.Provisioner
}

func NewProvisionerController(defaultProvisioner model.Provisioner, repo persistence.DeploymentRepository) *ProvisionerController {
	result := ProvisionerController{
		Repository:   repo,
		Provisioners: make(map[string]model.Provisioner),
	}

	result.Provisioners[baremetalProvisionerType] = defaultProvisioner
	result.Provisioners["kubernetes"] = kubernetes.NewKubernetesController()
	return &result
}

func (p *ProvisionerController) Provision(deploymentID, infraID, product string, args map[string][]string, framework string) (model.DeploymentInfo, error) {
	deployment, err := p.Repository.GetDeployment(deploymentID)
	if err != nil {
		log.WithError(err).Errorf("Error getting deployment %s", deploymentID)
		return deployment, err
	}

	infra, err := p.Repository.FindInfrastructure(deploymentID, infraID)
	if err != nil {
		log.WithError(err).Errorf("Error finding infrastructure %s", infraID)
		return deployment, err
	}

	for _, prod := range infra.Products {
		if prod == product {
			return deployment, fmt.Errorf("Product %s already present in the infrastructure", product)
		}
	}

	provType := framework
	if provType == "" {
		provType = baremetalProvisionerType
	}

	provisioner, ok := p.Provisioners[provType]
	if !ok {
		return deployment, fmt.Errorf("Can't find provisioner for framework %s", provType)
	}

	err = provisioner.Provision(deploymentID, &infra, product, args)
	if err != nil {
		log.WithError(err).Errorf("Error provisioning product %s", product)
		return deployment, err
	}

	return p.Repository.UpdateInfrastructure(deploymentID, infra)
}
