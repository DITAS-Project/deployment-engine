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

package ansible

import (
	"deployment-engine/model"
	"deployment-engine/utils"
	"fmt"

	"github.com/sirupsen/logrus"
)

const (
	KubernetesRegistriesSecretName = "docker-registries"
)

type RegistryProvisioner struct {
	parent        *Provisioner
	scriptsFolder string
}

func NewRegistryProvisioner(parent *Provisioner) RegistryProvisioner {
	return RegistryProvisioner{
		parent:        parent,
		scriptsFolder: parent.ScriptsFolder,
	}
}

func (p RegistryProvisioner) BuildInventory(deploymentID string, infra *model.InfrastructureDeploymentInfo, args model.Parameters) (Inventory, error) {
	return p.parent.Provisioners["kubeadm"].BuildInventory(deploymentID, infra, args)
}

func (p RegistryProvisioner) DeployProduct(inventoryPath, deploymentID string, infra *model.InfrastructureDeploymentInfo, args model.Parameters) error {

	logger := logrus.WithFields(map[string]interface{}{
		"deployment":     deploymentID,
		"infrastructure": infra.ID,
	})

	kubeConfigIn, ok := infra.Products["kubernetes"]
	if !ok {
		return fmt.Errorf("Kubernetes is not installed in infrastructure %s of deployment %s", infra.ID, deploymentID)
	}

	var kubeConfig KubernetesConfiguration
	err := utils.TransformObject(kubeConfigIn, &kubeConfig)
	if err != nil {
		return fmt.Errorf("Error getting kubernetes configuration: %s", err.Error())
	}

	repos := utils.GetDockerRepositories()

	for _, repo := range repos {
		args := map[string]string{
			"repo_name":     repo.Name,
			"repo_username": repo.Username,
			"repo_password": repo.Password,
		}

		if repo.Certificate != "" {
			args["cert_file"] = repo.Certificate
		}

		err = ExecutePlaybook(logger, p.scriptsFolder+"/kubernetes/docker_repository.yml", inventoryPath, args)
		if err != nil {
			return fmt.Errorf("Error configuring repository %s: %s", repo.Name, err.Error())
		}
	}

	if len(repos) > 0 {
		err = ExecutePlaybook(logger, p.scriptsFolder+"/kubernetes/docker_repository_secret.yml", inventoryPath, map[string]string{
			"secret_name": KubernetesRegistriesSecretName,
		})
		if err != nil {
			return fmt.Errorf("Error creating kubernetes secret for docker repositories: %s", err.Error())
		}
	}

	kubeConfig.RegistriesSecret = KubernetesRegistriesSecretName
	infra.Products["kubernetes"] = kubeConfig

	return nil
}
