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

import "deployment-engine/model"

//DeploymentRepository is the interface that must be implemented by persistence providers for deployments.
type DeploymentRepository interface {

	//Save a new deployment information and return the updated deployment from the underlying database
	Save(deployment model.DeploymentInfo) (model.DeploymentInfo, error)

	//Get the deployment information given its ID
	Get(deploymentID string) (model.DeploymentInfo, error)

	//List all available deployments
	List() ([]model.DeploymentInfo, error)

	//Update a deployment replacing its old contents
	Update(deployment model.DeploymentInfo) (model.DeploymentInfo, error)

	//Delete a deployment given its ID
	Delete(deploymentID string) error
}

type ProductRepository interface {
	//Save a new product information and return the created product from the underlying database
	Save(product model.Product) (model.Product, error)

	//Get the product information given its ID
	Get(productId string) (model.Product, error)

	//List all available products
	List() ([]model.Product, error)

	//Update a product replacing its old contents
	Update(deployment model.Product) (model.Product, error)

	//Delete a product given its ID
	Delete(productId string) error
}
