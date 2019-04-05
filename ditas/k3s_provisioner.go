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
 * This is being developed for the DITAS Project: https://www.ditas-project.eu/
 */

package ditas

import (
	"deployment-engine/model"
	"deployment-engine/provision/ansible"

	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/sirupsen/logrus"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type K3sProvisioner struct {
	parent        *ansible.Provisioner
	scriptsFolder string
	registry      Registry
}

type DockerJsonAuthType struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email"`
	Auth     string `json:"auth"`
}

type DockerJsonSecret struct {
	Auths map[string]DockerJsonAuthType `json:"auths"`
}

func NewK3sProvisioner(parent *ansible.Provisioner, scriptsFolder string, registry Registry) K3sProvisioner {
	return K3sProvisioner{
		parent:        parent,
		scriptsFolder: scriptsFolder,
		registry:      registry,
	}
}

func (p K3sProvisioner) BuildInventory(deploymentID string, infra model.InfrastructureDeploymentInfo, args map[string][]string) (ansible.Inventory, error) {
	return p.parent.Provisioners["kubeadm"].BuildInventory(deploymentID, infra, args)
}

func (p K3sProvisioner) DeployProduct(inventoryPath, deploymentID string, infra model.InfrastructureDeploymentInfo, args map[string][]string) error {

	logger := logrus.WithFields(logrus.Fields{
		"deployment":     deploymentID,
		"infrastructure": infra.ID,
	})

	inventoryFolder := p.parent.GetInventoryFolder(deploymentID, infra.ID)

	err := ansible.ExecutePlaybook(logger, p.scriptsFolder+"/deploy_k3s.yml", inventoryPath, map[string]string{
		"master_ip":        infra.Master.IP,
		"inventory_folder": inventoryFolder,
	})
	if err != nil {
		logger.WithError(err).Error("Error initializing master")
		return err
	}

	err = ansible.ExecutePlaybook(logger, p.scriptsFolder+"/join_k3s_nodes.yml", inventoryPath, map[string]string{
		"master_ip": infra.Master.IP,
	})
	if err != nil {
		logger.WithError(err).Error("Error joining workers to cluster")
		return err
	}

	if p.registry.URL != "" {
		jsonDockerAuth := DockerJsonSecret{
			Auths: map[string]DockerJsonAuthType{
				p.registry.URL: DockerJsonAuthType{
					Username: p.registry.Username,
					Password: p.registry.Password,
					Email:    p.registry.Email,
					Auth:     base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", p.registry.Username, p.registry.Password))),
				},
			},
		}
		strDockerAuth, err := json.Marshal(jsonDockerAuth)
		if err != nil {
			logger.WithError(err).Error("Error marshaling docker registry authorizaton")
			return err
		}

		encodedDockerAuth := base64.StdEncoding.EncodeToString(strDockerAuth)

		kubeclient, err := GetKubernetesClient(p.parent, deploymentID, infra.ID)
		if err != nil {
			logger.WithError(err).Error("Error getting kubernetes client")
			return err
		}

		kubeclient.AppsV1().Deployments(apiv1.NamespaceDefault)
		_, err = kubeclient.CoreV1().Secrets("default").Create(&apiv1.Secret{
			Type: apiv1.SecretTypeDockerConfigJson,
			Data: map[string][]byte{
				".dockerconfigjson": []byte(encodedDockerAuth),
			},
			ObjectMeta: v1.ObjectMeta{
				Name: p.registry.Name,
			},
		})
		if err != nil {
			logger.WithError(err).Errorf("Error creating private registry secret")
			return err
		}
	}

	return err
}
