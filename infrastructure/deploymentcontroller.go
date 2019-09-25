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

	log "github.com/sirupsen/logrus"
)

type InfrastructureCreationResult struct {
	Info  model.InfrastructureDeploymentInfo
	Error error
}

//Deployer is the main hybrid infrastructure deployer object
type Deployer struct {
	Repository    persistence.DeploymentRepository
	Vault         persistence.Vault
	PublicKeyPath string
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

	if c.PublicKeyPath == "" {
		return nil, errors.New("A public key location is needed to initialize a provider")
	}

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

		dep, err := cloudsigma.NewDeployer(provider.APIEndpoint, credentials, c.PublicKeyPath)
		if err != nil {
			return nil, fmt.Errorf("Error initializing deployer for %s: %w", provider.APIType, err)
		}
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

func (c *Deployer) DeployInfrastructure(infra model.InfrastructureType, channel chan InfrastructureCreationResult) {
	deployer, err := c.findProvider(infra.Provider)

	if err != nil {
		channel <- InfrastructureCreationResult{
			Error: err,
		}
		return
	}
	depInfo, err := deployer.DeployInfrastructure(infra)
	depInfo.Provider = infra.Provider
	channel <- InfrastructureCreationResult{
		Info:  depInfo,
		Error: err,
	}
	return
}

//CreateDeployment will create an hybrid deployment with the configuration passed as argument
func (c *Deployer) CreateDeployment(infras []model.InfrastructureType) ([]model.InfrastructureDeploymentInfo, error) {

	result := make([]model.InfrastructureDeploymentInfo, 0, len(infras))

	log.Tracef("Starting new deployment")

	channel := make(chan InfrastructureCreationResult, len(infras))

	for _, infra := range infras {
		go c.DeployInfrastructure(infra, channel)
	}

	var depError error
	for remaining := len(infras); remaining > 0; remaining-- {
		infraInfo := <-channel
		if infraInfo.Error != nil {
			log.WithError(infraInfo.Error).Errorf("Error creating infrastructure", infraInfo.Info.Name)
			depError = infraInfo.Error
		} else {
			infra, err := c.Repository.AddInfrastructure(infraInfo.Info)
			if err != nil {
				log.WithError(err).Error("Error adding infrastructure %s")
			}
			result = append(result, infra)
		}
	}

	if depError != nil {
		// TODO: Remove partial infrastructures if autoclean is on
	}

	return result, depError
}

func (c *Deployer) DeleteDeployment(infras []string) error {

	channel := make(chan InfrastructureCreationResult, len(infras))

	for _, infra := range infras {
		go c.DeleteInfrastructureParallel(infra, channel)
	}

	var depError error
	for remaining := len(infras); remaining > 0; remaining-- {
		result := <-channel
		if result.Error != nil {
			log.WithError(result.Error).Errorf("Error deleting infrastructure %s", result.Info.ID)
			depError = result.Error
		}
	}

	return depError

}

func (c *Deployer) DeleteInfrastructureParallel(infraID string, channel chan InfrastructureCreationResult) error {
	_, err := c.DeleteInfrastructure(infraID)
	channel <- InfrastructureCreationResult{
		Info: model.InfrastructureDeploymentInfo{
			ID: infraID,
		},
		Error: err,
	}
	return err
}

//DeleteInfrastructure will delete an infrastructure from a deployment. It will delete the deployment itself when there aren't infrastructures left.
func (c *Deployer) DeleteInfrastructure(infraID string) (model.InfrastructureDeploymentInfo, error) {

	infra, err := c.Repository.FindInfrastructure(infraID)
	if err != nil {
		log.WithError(err).Errorf("Infrastructure not found")
		return infra, err
	}

	deployer, err := c.findProvider(infra.Provider)
	if err != nil {
		log.WithError(err).Errorf("Can't find providers for infrastructure ID %s", infraID)
		return infra, err
	}

	delErrors := deployer.DeleteInfrastructure(infra)
	if delErrors != nil && len(delErrors) > 0 {
		for k, v := range delErrors {
			log.WithError(v).Errorf("Error deleting host %s", k)
		}
		return infra, fmt.Errorf("Errors found deleting infrastructure: %v", delErrors)
	}

	return c.Repository.DeleteInfrastructure(infraID)
}
