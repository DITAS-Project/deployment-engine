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

package persistence

import (
	"deployment-engine/model"
)

//DeploymentRepository is the interface that must be implemented by persistence providers for deployments.
type DeploymentRepository interface {

	//AddInfrastructure adds a new infrastructure to an existing deployment
	AddInfrastructure(infra model.InfrastructureDeploymentInfo) (model.InfrastructureDeploymentInfo, error)

	//UpdateInfrastructure updates as a whole an existing infrastructure in a deployment
	UpdateInfrastructure(infra model.InfrastructureDeploymentInfo) (model.InfrastructureDeploymentInfo, error)

	//FindInfrastructure finds an infrastructure in a deployment given their identifiers
	FindInfrastructure(infraID string) (model.InfrastructureDeploymentInfo, error)

	//DeleteInfrastructure will delete an infrastructure from a deployment given their identifiers
	DeleteInfrastructure(infraID string) (model.InfrastructureDeploymentInfo, error)

	// UpdateInfrastructureStatus updates the status of a infrastructure in a deployment
	UpdateInfrastructureStatus(infrastructureID, status string) (model.InfrastructureDeploymentInfo, error)

	// AddProductToInfrastructure adds a new product to an existing infrastructure
	AddProductToInfrastructure(infrastructureID, product string, configuration interface{}) (model.InfrastructureDeploymentInfo, error)
}

// Vault will be implemented by components that store authentication information. They can do so locally or they can be remote vaults like Hashicorp Vault.
type Vault interface {
	AddSecret(secret model.Secret) (string, error)
	UpdateSecret(secretID string, secret model.Secret) error
	GetSecret(secretID string) (model.Secret, error)
	DeleteSecret(secretID string) error
}
