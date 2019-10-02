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

	"github.com/sirupsen/logrus"
)

const (
	DockerPresentProperty       = "ansible_docker_installed"
	KubeadmPreinstalledProperty = "kubeadm_preinstalled_image"
)

type KubernetesProvisioner struct {
	parent *Provisioner
}

type KubernetesConfiguration struct {
	ConfigurationFile string
	RegistriesSecret  string
}

func NewKubernetesProvisioner(parent *Provisioner) *KubernetesProvisioner {
	return &KubernetesProvisioner{
		parent: parent,
	}
}

func (p KubernetesProvisioner) buildHost(host model.NodeInfo) InventoryHost {
	var role string
	if host.Role == "master" {
		role = "master"
	} else {
		role = "node"
	}

	return InventoryHost{
		Name: host.Hostname,
		Vars: map[string]string{
			"ansible_host":    host.IP,
			"ansible_user":    host.Username,
			"kubernetes_role": role,
		},
	}
}

func (p KubernetesProvisioner) BuildInventory(infra *model.InfrastructureDeploymentInfo, args model.Parameters) (Inventory, error) {
	return DefaultKubernetesInventory(*infra), nil
}

func (p KubernetesProvisioner) DeployProduct(inventoryPath string, infra *model.InfrastructureDeploymentInfo, args model.Parameters) (model.Parameters, error) {

	result := make(model.Parameters)
	logger := logrus.WithField("product", "kubernetes").WithField("infrastructure", infra.ID)

	if infra.ExtraProperties.GetBool(KubeadmPreinstalledProperty) {

		err := ExecutePlaybook(logger, p.parent.ScriptsFolder+"/kubernetes/kubeadm.yml", inventoryPath, nil)
		if err != nil {
			return result, err
		}

	} else {

		if !infra.ExtraProperties.GetBool(DockerPresentProperty) {
			args["wait"] = []string{"false"}
			out, err := p.parent.Provision(infra, "docker", args)
			if err != nil {
				return out, err
			}
			result.AddAll(out)
		}

		logger := logrus.WithField("product", "kubernetes")
		err := utils.ExecuteCommand(logger, "ansible-galaxy", "install", "geerlingguy.kubernetes")
		if err != nil {
			return result, err
		}

		err = ExecutePlaybook(logger, p.parent.ScriptsFolder+"/kubernetes/main.yml", inventoryPath, nil)
		if err != nil {
			return result, err
		}
	}

	inventoryFolder := p.parent.GetInventoryFolder(infra.ID)
	err := ExecutePlaybook(logger, p.parent.ScriptsFolder+"/kubernetes/get_k8s_config.yml", inventoryPath, map[string]string{
		"inventory_folder": inventoryFolder,
	})

	if err != nil {
		return result, utils.WrapLogAndReturnError(logger, "Error getting k8s configuration", err)
	}

	infra.Products["kubernetes"] = KubernetesConfiguration{
		ConfigurationFile: inventoryFolder + "/config",
	}

	repos := utils.GetDockerRepositories()
	if repos != nil && len(repos) > 0 {
		args[AnsibleWaitForSSHReadyProperty] = []string{"false"}
		out, err := p.parent.Provision(infra, "private_registries", args)
		if err != nil {
			return result, err
		}
		result.AddAll(out)
	}

	return result, nil
}
