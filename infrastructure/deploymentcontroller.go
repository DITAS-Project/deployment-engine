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
 */

package infrastructure

import (
	"deployment-engine/infrastructure/cloudsigma"
	"deployment-engine/model"
	"deployment-engine/persistence"
	"deployment-engine/utils"
	"fmt"
	"strings"

	"github.com/google/uuid"

	log "github.com/sirupsen/logrus"
)

//Deployer is the main hybrid infrastructure deployer object
type Deployer struct {
	Repository persistence.DeploymentRepository
	Vault      persistence.Vault
}

func (c *Deployer) findProvider(provider model.CloudProviderInfo) (model.Deployer, error) {

	if provider.SecretID == "" {
		return nil, fmt.Errorf("Secret identifier is empty for Cloud Provider %v", provider)
	}

	if strings.ToLower(provider.APIType) == "cloudsigma" {

		var credentials persistence.BasicAuthSecret
		err := c.Vault.GetSecret(provider.SecretID, &credentials)
		if err != nil {
			return nil, err
		}

		dep, err := cloudsigma.NewDeployer(provider.APIEndpoint, credentials)
		return *dep, err
	}

	return nil, fmt.Errorf("Can't find a suitable deployer for API type %s", provider.APIType)
}

//CreateDeployment will create an hybrid deployment with the configuration passed as argument
func (c *Deployer) CreateDeployment(deployment model.Deployment) (model.DeploymentInfo, error) {

	result := model.DeploymentInfo{
		ID:              uuid.New().String(),
		Status:          "starting",
		Infrastructures: make([]model.InfrastructureDeploymentInfo, 0),
	}

	result, err := c.Repository.SaveDeployment(result)

	if err != nil {
		log.WithError(err).Error("Error inserting deployment in the database")
		return result, err
	}

	logger := log.WithField("deployment", result.ID)

	logger.Tracef("Starting new deployment")

	for _, infra := range deployment.Infrastructure {
		logger = logger.WithField("infrastructure", infra.Name)
		deployer, infraErr := c.findProvider(infra.Provider)

		if infraErr != nil {
			logger.WithError(infraErr).Errorf("Error getting deployment provider")
			break
		}

		infraDeployment, infraErr := deployer.DeployInfrastructure(infra)
		if infraErr != nil {
			logger.WithError(infraErr).Error("Error deploying infrastructure")
		}

		if infraDeployment.ID != "" {
			infraDeployment.Provider = infra.Provider
			result.Infrastructures = append(result.Infrastructures, infraDeployment)
			result, infraErr = c.Repository.UpdateDeployment(result)
			if infraErr != nil {
				logger.WithError(infraErr).Error("Error updating deployment status")
			}
		}
	}

	return result, err
}

//DeleteInfrastructure will delete an infrastructure from a deployment. It will delete the deployment itself when there aren't infrastructures left.
func (c *Deployer) DeleteInfrastructure(deploymentID, infraID string) (model.DeploymentInfo, error) {
	deployment, err := c.Repository.GetDeployment(deploymentID)
	if err != nil {
		log.WithError(err).Errorf("Deployment ID %s not found", deploymentID)
		return model.DeploymentInfo{}, err
	}

	index, infra, err := utils.FindInfra(deployment, infraID)
	if err != nil {
		log.WithError(err).Errorf("Infrastructure not found")
		return deployment, err
	}

	deployer, err := c.findProvider(infra.Provider)
	if err != nil {
		log.WithError(err).Errorf("Can't find providers for infrastructure ID %s", infraID)
		return deployment, err
	}

	delErrors := deployer.DeleteInfrastructure(*infra)
	if delErrors != nil && len(delErrors) > 0 {
		for k, v := range delErrors {
			log.WithError(v).Errorf("Error deleting host %s", k)
		}
		return deployment, fmt.Errorf("Errors found deleting infrastructure: %v", delErrors)
	}

	deployment.Infrastructures = c.remove(deployment.Infrastructures, index)
	if len(deployment.Infrastructures) == 0 {
		err = c.Repository.DeleteDeployment(deployment.ID)
		if err != nil {
			log.WithError(err).Errorf("Error deleting deployment ID %s", deployment.ID)
			return deployment, err
		}
		return model.DeploymentInfo{}, nil
	}

	deployment, err = c.Repository.UpdateDeployment(deployment)
	if err != nil {
		log.WithError(err).Errorf("Error updating deployment ID %s", deployment.ID)
		return deployment, err
	}

	return deployment, nil

}

func (c *Deployer) remove(s []model.InfrastructureDeploymentInfo, i int) []model.InfrastructureDeploymentInfo {
	if i < len(s) {
		s[i] = s[len(s)-1]
		return s[:len(s)-1]
	}
	return s
}
