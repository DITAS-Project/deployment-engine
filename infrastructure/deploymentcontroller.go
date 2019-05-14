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
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	log "github.com/sirupsen/logrus"
)

type InfrastructureCreationResult struct {
	Info  model.InfrastructureDeploymentInfo
	Error error
}

//Deployer is the main hybrid infrastructure deployer object
type Deployer struct {
	Repository persistence.DeploymentRepository
	Vault      persistence.Vault
}

func (c *Deployer) transformCredentials(raw, result interface{}) error {
	strValue, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(strValue, result)
}

func (c *Deployer) getProviderCredentials(provider model.CloudProviderInfo, credentials interface{}) error {
	if provider.Credentials != nil {
		return c.transformCredentials(provider.Credentials, credentials)
	}

	if provider.SecretID != "" {
		if c.Vault == nil {
			return errors.New("Found secret identifier but a vault hasn't been configured")
		}
		secret, err := c.Vault.GetSecret(provider.SecretID)
		if err != nil {
			return err
		}
		return c.transformCredentials(secret.Content, credentials)
	}

	return errors.New("Secret ID or credentials are needed for cloud provider")
}

func (c *Deployer) findProvider(provider model.CloudProviderInfo) (model.Deployer, error) {

	if provider.SecretID == "" && (provider.Credentials == nil || len(provider.Credentials) == 0) {
		return nil, fmt.Errorf("Either secret ID or provider credentials are mandatory for Provider %v", provider)
	}

	if strings.ToLower(provider.APIType) == "cloudsigma" {

		var credentials model.BasicAuthSecret
		err := c.getProviderCredentials(provider, &credentials)

		if err != nil {
			return nil, err
		}

		if credentials.Username == "" || credentials.Password == "" {
			return nil, fmt.Errorf("Invalid credentials specified for cloudsigma provider %s. Username and password are needed", provider.APIEndpoint)
		}

		dep, err := cloudsigma.NewDeployer(provider.APIEndpoint, credentials)
		return *dep, err
	}

	return nil, fmt.Errorf("Can't find a suitable deployer for API type %s", provider.APIType)
}

func (c *Deployer) mergeCustomProperties(source map[string]string, target map[string]string) map[string]string {
	result := make(map[string]string)
	if source != nil {
		result = source
	}
	if target != nil {
		for k, v := range target {
			result[k] = v
		}
	}
	return result
}

func (c *Deployer) DeployInfrastructure(deploymentID string, infra model.InfrastructureType, channel chan InfrastructureCreationResult) {
	deployer, err := c.findProvider(infra.Provider)

	if err != nil {
		channel <- InfrastructureCreationResult{
			Error: err,
		}
		return
	}
	depInfo, err := deployer.DeployInfrastructure(deploymentID, infra)
	depInfo.Provider = infra.Provider
	channel <- InfrastructureCreationResult{
		Info:  depInfo,
		Error: err,
	}
	return
}

//CreateDeployment will create an hybrid deployment with the configuration passed as argument
func (c *Deployer) CreateDeployment(deployment model.Deployment) (model.DeploymentInfo, error) {

	result := model.DeploymentInfo{
		ID:              uuid.New().String(),
		Name:            deployment.Name,
		Status:          "starting",
		Infrastructures: make(map[string]model.InfrastructureDeploymentInfo),
	}

	result, err := c.Repository.SaveDeployment(result)

	if err != nil {
		log.WithError(err).Error("Error inserting deployment in the database")
		return result, err
	}

	logger := log.WithField("deployment", result.ID)

	logger.Tracef("Starting new deployment")

	channel := make(chan InfrastructureCreationResult, len(deployment.Infrastructures))

	for _, infra := range deployment.Infrastructures {
		go c.DeployInfrastructure(result.ID, infra, channel)
	}

	var depError error
	for remaining := len(deployment.Infrastructures); remaining > 0; remaining-- {
		infraInfo := <-channel
		if infraInfo.Error != nil {
			logger.WithError(err).Error("Error creating infrastructure")
			depError = infraInfo.Error
		} else {
			infraDeployment := infraInfo.Info
			if infraDeployment.ID != "" {
				result, err = c.Repository.AddInfrastructure(result.ID, infraInfo.Info)
				if err != nil {
					logger.WithError(err).Error("Error adding infrastructure")
				}
			} else {
				depError = errors.New("Infrastructure created without an identifier")
				logger.WithError(err).Error("Error creating infrastructure")
			}
		}
	}

	if depError != nil {
		// TODO: Remove partial infrastructures if autoclean is on
	}

	return result, depError
}

func (c *Deployer) DeleteDeployment(deploymentID string) error {
	deployment, err := c.Repository.GetDeployment(deploymentID)
	if err != nil {
		return err
	}

	channel := make(chan InfrastructureCreationResult, len(deployment.Infrastructures))

	for _, infra := range deployment.Infrastructures {
		go c.DeleteInfrastructureParallel(deployment.ID, infra.ID, channel)
	}

	var depError error
	for remaining := len(deployment.Infrastructures); remaining > 0; remaining-- {
		result := <-channel
		if result.Error != nil {
			log.WithError(err).Errorf("Error deleting infrastructure %s", result.Info.ID)
		}
	}

	if depError == nil {
		return c.Repository.DeleteDeployment(deploymentID)
	}

	return depError

}

func (c *Deployer) DeleteInfrastructureParallel(deploymentID, infraID string, channel chan InfrastructureCreationResult) error {
	_, err := c.DeleteInfrastructure(deploymentID, infraID)
	channel <- InfrastructureCreationResult{
		Info: model.InfrastructureDeploymentInfo{
			ID: infraID,
		},
		Error: err,
	}
	return err
}

//DeleteInfrastructure will delete an infrastructure from a deployment. It will delete the deployment itself when there aren't infrastructures left.
func (c *Deployer) DeleteInfrastructure(deploymentID, infraID string) (model.DeploymentInfo, error) {
	deployment, err := c.Repository.GetDeployment(deploymentID)
	if err != nil {
		log.WithError(err).Errorf("Deployment ID %s not found", deploymentID)
		return model.DeploymentInfo{}, err
	}

	infra, err := c.Repository.FindInfrastructure(deploymentID, infraID)
	if err != nil {
		log.WithError(err).Errorf("Infrastructure not found")
		return deployment, err
	}

	deployer, err := c.findProvider(infra.Provider)
	if err != nil {
		log.WithError(err).Errorf("Can't find providers for infrastructure ID %s", infraID)
		return deployment, err
	}

	delErrors := deployer.DeleteInfrastructure(deploymentID, infra)
	if delErrors != nil && len(delErrors) > 0 {
		for k, v := range delErrors {
			log.WithError(v).Errorf("Error deleting host %s", k)
		}
		return deployment, fmt.Errorf("Errors found deleting infrastructure: %v", delErrors)
	}

	return c.Repository.DeleteInfrastructure(deploymentID, infraID)
}
