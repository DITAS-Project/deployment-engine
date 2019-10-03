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
	"time"

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
	infrastructures map[string]model.InfrastructureDeploymentInfo
	vault           map[string]model.Secret
}

func CreateMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		infrastructures: make(map[string]model.InfrastructureDeploymentInfo),
		vault:           make(map[string]model.Secret),
	}
}

//UpdateInfrastructure updates as a whole an existing infrastructure in a deployment
func (m *MemoryRepository) UpdateInfrastructure(infra model.InfrastructureDeploymentInfo) (model.InfrastructureDeploymentInfo, error) {

	if infra.ID == "" {
		return model.InfrastructureDeploymentInfo{}, errors.New("Trying to update infrastructure without identifier")
	}
	infra.UpdateTime = time.Now()
	m.infrastructures[infra.ID] = infra
	return infra, nil
}

// UpdateInfrastructureStatus updates the status of a infrastructure in a deployment
func (m *MemoryRepository) UpdateInfrastructureStatus(infrastructureID, status string) (model.InfrastructureDeploymentInfo, error) {
	infra, err := m.FindInfrastructure(infrastructureID)
	if err != nil {
		return infra, err
	}

	infra.Status = status
	return m.UpdateInfrastructure(infra)
}

//AddInfrastructure adds a new infrastructure to an existing deployment
func (m *MemoryRepository) AddInfrastructure(infra model.InfrastructureDeploymentInfo) (model.InfrastructureDeploymentInfo, error) {
	if infra.ID == "" {
		infra.ID = uuid.New().String()
	}
	infra.CreationTime = time.Now()
	return m.UpdateInfrastructure(infra)
}

//FindInfrastructure finds an infrastructure in a deployment given their identifiers
func (m *MemoryRepository) FindInfrastructure(infraID string) (model.InfrastructureDeploymentInfo, error) {

	infra, ok := m.infrastructures[infraID]
	if !ok {
		return infra, fmt.Errorf("Can't find infrastructure with identifier %s", infraID)
	}
	return infra, nil
}

//DeleteInfrastructure will delete an infrastructure from a deployment given their identifiers
func (m *MemoryRepository) DeleteInfrastructure(infraID string) (model.InfrastructureDeploymentInfo, error) {
	infra, err := m.FindInfrastructure(infraID)
	if err != nil {
		return infra, err
	}

	delete(m.infrastructures, infraID)
	return infra, nil
}

// AddProductToInfrastructure adds a new product to an existing infrastructure
func (m *MemoryRepository) AddProductToInfrastructure(infrastructureID, product string, config interface{}) (model.InfrastructureDeploymentInfo, error) {

	infra, err := m.FindInfrastructure(infrastructureID)
	if err != nil {
		return infra, err
	}

	if infra.Products == nil {
		infra.Products = make(map[string]interface{})
	}

	infra.Products[product] = config
	return m.UpdateInfrastructure(infra)
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
		return secret, fmt.Errorf("Can't find secret %s", secretID)
	}
	return secret, nil
}

// DeleteSecret deletes a secret from the vault given its identifier
func (v *MemoryRepository) DeleteSecret(secretID string) error {
	delete(v.vault, secretID)
	return nil
}
