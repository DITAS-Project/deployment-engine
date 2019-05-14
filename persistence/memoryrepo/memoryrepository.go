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

package memoryrepo

import (
	"deployment-engine/model"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

const (
	deploymentType = "Deployment"
	productType    = "Product"
	secretType     = "Secret"
)

// MemoryRepository implements a repository in memory
// WARNING: When used as vault, it stores credentials and private keys in memory and UNENCRYPTED which is a VERY bad practice and it's strongly discouraged to be used in production. Use it for development and test but change later for a secure vault implementation.
type MemoryRepository struct {
	deployments map[string]model.DeploymentInfo
	vault       map[string]model.Secret
}

func CreateMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		deployments: make(map[string]model.DeploymentInfo),
		vault:       make(map[string]model.Secret),
	}
}

//SaveDeployment a new deployment information and return the updated deployment from the underlying database
func (m *MemoryRepository) SaveDeployment(deployment model.DeploymentInfo) (model.DeploymentInfo, error) {
	deployment.ID = uuid.New().String()
	m.deployments[deployment.ID] = deployment
	return deployment, nil
}

//UpdateDeployment a deployment replacing its old contents
func (m *MemoryRepository) UpdateDeployment(deployment model.DeploymentInfo) (model.DeploymentInfo, error) {
	_, err := m.GetDeployment(deployment.ID)
	if err == nil {
		m.deployments[deployment.ID] = deployment
	}
	return deployment, err
}

//GetDeployment the deployment information given its ID
func (m *MemoryRepository) GetDeployment(deploymentID string) (model.DeploymentInfo, error) {
	dep, ok := m.deployments[deploymentID]
	if !ok {
		return dep, fmt.Errorf("Can't find deployment %s", deploymentID)
	}
	return dep, nil
}

//ListDeployment all available deployments
func (m *MemoryRepository) ListDeployment() ([]model.DeploymentInfo, error) {
	result := make([]model.DeploymentInfo, len(m.deployments))
	i := 0
	for _, v := range m.deployments {
		result[i] = v
		i++
	}
	return result, nil
}

//DeleteDeployment a deployment given its ID
func (m *MemoryRepository) DeleteDeployment(deploymentID string) error {
	delete(m.deployments, deploymentID)
	return nil
}

// UpdateDeploymentStatus updates the status of a deployment
func (m *MemoryRepository) UpdateDeploymentStatus(deploymentID, status string) (model.DeploymentInfo, error) {
	updated, err := m.GetDeployment(deploymentID)
	if err != nil {
		return updated, err
	}
	updated.Status = status
	return m.UpdateDeployment(updated)
}

//UpdateInfrastructure updates as a whole an existing infrastructure in a deployment
func (m *MemoryRepository) UpdateInfrastructure(deploymentID string, infra model.InfrastructureDeploymentInfo) (model.DeploymentInfo, error) {

	var dep model.DeploymentInfo

	if infra.ID == "" {
		return dep, errors.New("Infrastructure has an empty ID")
	}

	dep, err := m.GetDeployment(deploymentID)
	if err != nil {
		return dep, fmt.Errorf("Can't find deployment %s: %s", deploymentID, err.Error())
	}

	if dep.Infrastructures == nil {
		dep.Infrastructures = make(map[string]model.InfrastructureDeploymentInfo)
	}

	dep.Infrastructures[infra.ID] = infra
	return m.UpdateDeployment(dep)
}

// UpdateInfrastructureStatus updates the status of a infrastructure in a deployment
func (m *MemoryRepository) UpdateInfrastructureStatus(deploymentID, infrastructureID, status string) (model.DeploymentInfo, error) {
	var updated model.DeploymentInfo

	infra, err := m.FindInfrastructure(deploymentID, infrastructureID)
	if err != nil {
		return updated, err
	}

	infra.Status = status

	return m.UpdateInfrastructure(deploymentID, infra)
}

//AddInfrastructure adds a new infrastructure to an existing deployment
func (m *MemoryRepository) AddInfrastructure(deploymentID string, infra model.InfrastructureDeploymentInfo) (model.DeploymentInfo, error) {
	return m.UpdateInfrastructure(deploymentID, infra)
}

//FindInfrastructure finds an infrastructure in a deployment given their identifiers
func (m *MemoryRepository) FindInfrastructure(deploymentID, infraID string) (model.InfrastructureDeploymentInfo, error) {

	dep, err := m.GetDeployment(deploymentID)
	if err != nil {
		return model.InfrastructureDeploymentInfo{}, err
	}

	infra, ok := dep.Infrastructures[infraID]
	if !ok {
		return infra, fmt.Errorf("Can't find infrastructure %s in deployment %s", infraID, deploymentID)
	}

	return infra, nil

}

//DeleteInfrastructure will delete an infrastructure from a deployment given their identifiers
func (m *MemoryRepository) DeleteInfrastructure(deploymentID, infraID string) (model.DeploymentInfo, error) {
	dep, err := m.GetDeployment(deploymentID)
	if err != nil {
		return dep, err
	}

	delete(dep.Infrastructures, infraID)
	return m.UpdateDeployment(dep)
}

// AddProductToInfrastructure adds a new product to an existing infrastructure
func (m *MemoryRepository) AddProductToInfrastructure(deploymentID, infrastructureID, product string, config interface{}) (model.DeploymentInfo, error) {

	infra, err := m.FindInfrastructure(deploymentID, infrastructureID)
	if err != nil {
		return model.DeploymentInfo{}, err
	}

	if infra.Products == nil {
		infra.Products = make(map[string]interface{})
	}

	infra.Products[product] = config
	return m.UpdateInfrastructure(deploymentID, infra)
}

// AddSecret adds a new secret to the vault, returning its identifier
func (v *MemoryRepository) AddSecret(secret model.Secret) (string, error) {
	id := uuid.New().String()
	v.vault[id] = secret
	return id, nil
}

// UpdateSecret updates a secret replacing its content if it exists or returning an error if not
func (v *MemoryRepository) UpdateSecret(secretID string, secret model.Secret) error {
	_, ok := v.vault[secretID]

	if !ok {
		return fmt.Errorf("Secret with identifier %s not found", secretID)
	}

	v.vault[secretID] = secret

	return nil
}

// GetSecret gets a secret information given its identifier
func (v *MemoryRepository) GetSecret(secretID string) (model.Secret, error) {
	secret, ok := v.vault[secretID]
	if !ok {
		return secret, fmt.Errorf("Can't find secret %", secretID)
	}
	return secret, nil
}

// DeleteSecret deletes a secret from the vault given its identifier
func (v *MemoryRepository) DeleteSecret(secretID string) error {
	delete(v.vault, secretID)
	return nil
}
