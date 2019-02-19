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
	"encoding/json"
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
	products    map[string]model.Product
	vault       map[string][]byte
}

func CreateMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		deployments: make(map[string]model.DeploymentInfo),
		products:    make(map[string]model.Product),
		vault:       make(map[string][]byte),
	}
}

func (v *MemoryRepository) get(ID, objectType string, result interface{}) error {
	var ok bool
	switch objectType {
	case deploymentType:
		*result.(*model.DeploymentInfo), ok = v.deployments[ID]
	case productType:
		*result.(*model.Product), ok = v.products[ID]
	case secretType:
		serialized, ok := v.vault[ID]
		if ok {
			return json.Unmarshal(serialized, result)
		}
	default:
		return fmt.Errorf("Unrecognized object type %s", objectType)
	}

	if !ok {
		return fmt.Errorf("%s %s not found in repository", objectType, ID)
	}
	return nil
}

//SaveDeployment a new deployment information and return the updated deployment from the underlying database
func (m *MemoryRepository) SaveDeployment(deployment model.DeploymentInfo) (model.DeploymentInfo, error) {
	deployment.ID = uuid.New().String()
	m.deployments[deployment.ID] = deployment
	return deployment, nil
}

//UpdateDeployment a deployment replacing its old contents
func (m *MemoryRepository) UpdateDeployment(deployment model.DeploymentInfo) (model.DeploymentInfo, error) {
	var dep model.DeploymentInfo
	err := m.get(deployment.ID, deploymentType, &dep)
	if err == nil {
		m.deployments[deployment.ID] = deployment
	}
	return deployment, err
}

//GetDeployment the deployment information given its ID
func (m *MemoryRepository) GetDeployment(deploymentID string) (model.DeploymentInfo, error) {
	var deployment model.DeploymentInfo
	err := m.get(deploymentID, deploymentType, &deployment)
	return deployment, err
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
func (m *MemoryRepository) UpdateDeploymentStatus(deploymentID, status string) error {
	var updated model.DeploymentInfo
	err := m.get(deploymentID, deploymentType, &updated)
	if err == nil {
		updated.Status = status
		m.deployments[deploymentID] = updated
	}
	return err
}

// UpdateInfrastructureStatus updates the status of a infrastructure in a deployment
func (m *MemoryRepository) UpdateInfrastructureStatus(deploymentID, infrastructureID, status string) error {
	var updated model.DeploymentInfo
	err := m.get(deploymentID, deploymentType, &updated)
	if err == nil {
		found := false
		for i := 0; i < len(updated.Infrastructures) && !found; i++ {
			if updated.Infrastructures[i].ID == infrastructureID {
				updated.Infrastructures[i].Status = status
				found = true
			}
		}
		m.deployments[deploymentID] = updated
	}
	return err
}

//SaveProduct a new product information and return the created product from the underlying database
func (m *MemoryRepository) SaveProduct(product model.Product) (model.Product, error) {
	product.ID = uuid.New().String()
	m.products[product.ID] = product
	return product, nil
}

//GetProduct the product information given its ID
func (m *MemoryRepository) GetProduct(productID string) (model.Product, error) {
	var result model.Product
	err := m.get(productID, productType, &result)
	return result, err
}

//ListProducts all available products
func (m *MemoryRepository) ListProducts() ([]model.Product, error) {
	result := make([]model.Product, len(m.products))
	i := 0
	for _, v := range m.products {
		result[i] = v
		i++
	}
	return result, nil
}

//UpdateProduct a product replacing its old contents
func (m *MemoryRepository) UpdateProduct(product model.Product) (model.Product, error) {
	var result model.Product
	err := m.get(product.ID, productType, &result)
	if err == nil {
		m.products[product.ID] = product
	}
	return product, err
}

//DeleteProduct a product given its ID
func (m *MemoryRepository) DeleteProduct(productID string) error {
	delete(m.products, productID)
	return nil
}

func (v *MemoryRepository) replaceSecret(ID string, secret interface{}) error {
	serialized, err := json.Marshal(secret)
	if err == nil {
		v.vault[ID] = serialized
	}
	return err
}

// AddSecret adds a new secret to the vault, returning its identifier
func (v *MemoryRepository) AddSecret(secret interface{}) (string, error) {
	id := uuid.New().String()
	return id, v.replaceSecret(id, secret)
}

// UpdateSecret updates a secret replacing its content if it exists or returning an error if not
func (v *MemoryRepository) UpdateSecret(secretID string, secret interface{}) error {
	_, ok := v.vault[secretID]

	if !ok {
		return fmt.Errorf("Secret with identifier %s not found", secretID)
	}

	return v.replaceSecret(secretID, secret)
}

// GetSecret gets a secret information given its identifier
func (v *MemoryRepository) GetSecret(secretID string, secretOut interface{}) error {
	return v.get(secretID, secretType, secretOut)
}

// DeleteSecret deletes a secret from the vault given its identifier
func (v *MemoryRepository) DeleteSecret(secretID string) error {
	delete(v.vault, secretID)
	return nil
}
