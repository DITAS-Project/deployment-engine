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
package main

import (
	"deployment-engine/model"
	"deployment-engine/persistence"
	"deployment-engine/persistence/memoryrepo"
	"deployment-engine/persistence/mongorepo"
	"deployment-engine/provision/ansible"
	"deployment-engine/restfrontend"
	"deployment-engine/utils"
	"fmt"

	"github.com/spf13/viper"

	log "github.com/sirupsen/logrus"
)

const (
	RepositoryProperty   = "repository.type"
	VaultProperty        = "vault.type"
	ProvisionerProperty  = "provisioner.type"
	FrontendProperty     = "frontent.type"
	FrontendPortProperty = "frontend.port"

	RepositoryDefault   = "mongo"
	VaultDefault        = "mongo"
	ProvisionerDefault  = "ansible"
	FrontendDefault     = "default"
	FrontendPortDefault = "8080"
)

func main() {
	viper.SetDefault(RepositoryProperty, RepositoryDefault)
	viper.SetDefault(ProvisionerProperty, ProvisionerDefault)
	viper.SetDefault(FrontendProperty, FrontendDefault)
	viper.SetDefault(FrontendPortProperty, FrontendPortDefault)

	configFolder, err := utils.ConfigurationFolder()
	if err != nil {
		log.WithError(err).Error("Error getting configuration folder")
		return
	}

	viper.AddConfigPath(configFolder)
	viper.SetConfigName("config")
	viper.ReadInConfig()

	repository, err := getRepository(viper.GetString(RepositoryProperty))
	if err != nil {
		log.WithError(err).Error("Error getting repository")
		return
	}

	vault, err := getVault(viper.GetString(VaultProperty), viper.GetString(RepositoryProperty), repository)
	if err != nil {
		log.WithError(err).Error("Error getting vault")
	}

	provisioner, err := getProvisioner(viper.GetString(ProvisionerProperty))
	if err != nil {
		log.WithError(err).Error("Error getting provisioner")
		return
	}

	frontend, err := getFrontend(viper.GetString(FrontendProperty), repository, vault, provisioner)
	if err != nil {
		log.WithError(err).Error("Error getting frontend")
		return
	}

	frontend.Run(":" + viper.GetString(FrontendPortProperty))
}

func getRepository(repoType string) (persistence.DeploymentRepository, error) {
	switch repoType {
	case "mongo":
		return mongorepo.CreateRepositoryNative()
	}
	return nil, fmt.Errorf("Unrecognized repository type %s", repoType)
}

func getVault(vaultType, repoType string, repo persistence.DeploymentRepository) (persistence.Vault, error) {
	switch vaultType {
	case "mongo":
		if repoType == "mongo" {
			return repo.(*mongorepo.MongoRepository), nil
		}
		return mongorepo.CreateRepositoryNative()
	case "memory":
		if repoType == "memory" {
			return repo.(*memoryrepo.MemoryRepository), nil
		}
		return memoryrepo.CreateMemoryRepository(), nil
	}
	return nil, fmt.Errorf("Unrecognized vault type %s", repoType)
}

func getProvisioner(provisionerType string) (model.Provisioner, error) {
	switch provisionerType {
	case "ansible":
		return ansible.New()
	}
	return nil, fmt.Errorf("Unrecognized provisioner type %s", provisionerType)
}

func getFrontend(frontendType string, repo persistence.DeploymentRepository, vault persistence.Vault, provisioner model.Provisioner) (model.Frontend, error) {
	switch frontendType {
	case "default":
		return restfrontend.New(repo, vault, provisioner), nil
	}
	return nil, fmt.Errorf("Unrecognized frontend type %s", frontendType)
}
