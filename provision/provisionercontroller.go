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

func (p *ProvisionerController) Provision(infraID, product string, args model.Parameters, framework string) (model.InfrastructureDeploymentInfo, error) {

	infra, err := p.Repository.FindInfrastructure(infraID)
	if err != nil {
		log.WithError(err).Errorf("Error finding infrastructure %s", infraID)
		return infra, err
	}

	for _, prod := range infra.Products {
		if prod == product {
			return infra, fmt.Errorf("Product %s already present in the infrastructure", product)
		}
	}

	provType := framework
	if provType == "" {
		provType = baremetalProvisionerType
	}

	provisioner, ok := p.Provisioners[provType]
	if !ok {
		return infra, fmt.Errorf("Can't find provisioner for framework %s", provType)
	}

	if args == nil {
		args = make(model.Parameters)
	}

	err = provisioner.Provision(&infra, product, args)
	if err != nil {
		log.WithError(err).Errorf("Error provisioning product %s", product)
		return infra, err
	}

	return p.Repository.UpdateInfrastructure(infra)
}
