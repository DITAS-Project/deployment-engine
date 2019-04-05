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

	blueprint "github.com/DITAS-Project/blueprint-go"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	DitasNamespace        = "default"
	DitasVDMConfigMapName = "vdm"
)

type VDMProvisioner struct {
	parent              *ansible.Provisioner
	scriptsFolder       string
	configVariablesPath string
	configFolder        string
	registry            Registry
}

func NewVDMProvisioner(parent *ansible.Provisioner, scriptsFolder, configVariablesPath, configFolder string, registry Registry) VDMProvisioner {
	return VDMProvisioner{
		parent:              parent,
		scriptsFolder:       scriptsFolder,
		configVariablesPath: configVariablesPath,
		configFolder:        configFolder,
		registry:            registry,
	}
}

func (p VDMProvisioner) BuildInventory(deploymentID string, infra model.InfrastructureDeploymentInfo, args map[string][]string) (ansible.Inventory, error) {
	return p.parent.Provisioners["kubeadm"].BuildInventory(deploymentID, infra, args)
}

func (p VDMProvisioner) DeployProduct(inventoryPath, deploymentID string, infra model.InfrastructureDeploymentInfo, args map[string][]string) error {

	logger := logrus.WithFields(logrus.Fields{
		"deployment":     deploymentID,
		"infrastructure": infra.ID,
	})

	configMap, err := GetConfigMapFromFolder(p.configFolder+"/vdm", DitasVDMConfigMapName)
	if err != nil {
		logger.WithError(err).Error("Error reading configuration map")
		return err
	}

	kubernetesClient, err := GetKubernetesClient(p.parent, deploymentID, infra.ID)
	if err != nil {
		logger.WithError(err).Error("Error getting kubernetes client")
		return err
	}

	configMapsClient := kubernetesClient.CoreV1().ConfigMaps(DitasNamespace)

	logger.Info("Creating or updating VDM config map")
	err = CreateOrUpdateResource(logger.WithFields(logrus.Fields{
		"resource": "ConfigMap",
		"name":     "vdm",
	}), DitasVDMConfigMapName,
		func(name string) (bool, error) {
			existing, err := configMapsClient.Get(name, metav1.GetOptions{})
			return err == nil && existing != nil && existing.Name == DitasVDMConfigMapName, err
		},
		configMapsClient.Delete,
		func(name string) error {
			_, err = configMapsClient.Create(&configMap)
			return err
		})

	if err != nil {
		return err
	}

	vdmLabels := map[string]string{
		"component": "vdm",
	}

	imageSet := make(blueprint.ImageSet)
	imageSet["ds4m"] = blueprint.ImageInfo{
		Image:        "ditas/decision-system-for-data-and-computation-movement",
		InternalPort: 8080,
	}

	vdmDeployment := GetPodDescription("vdm", int32(1), int64(30), vdmLabels, imageSet, DitasVDMConfigMapName)

	podClient := kubernetesClient.AppsV1().Deployments(DitasNamespace)

	logger.Info("Creating or updating VDM pod")
	err = CreateOrUpdateResource(logger.WithFields(logrus.Fields{
		"resource": "Pod",
		"name":     "vdm",
	}), "vdm",
		func(name string) (bool, error) {
			existing, err := podClient.Get(name, metav1.GetOptions{})
			return err == nil && existing != nil && existing.Name == DitasVDMConfigMapName, err
		},
		podClient.Delete,
		func(string) error {
			_, err = podClient.Create(&vdmDeployment)
			return err
		})

	if err != nil {
		return err
	}

	vdmService := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "vdm",
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: vdmLabels,
			Ports: []corev1.ServicePort{
				corev1.ServicePort{
					Name: "ds4m",
					Port: 8080,
				},
			},
		},
	}

	serviceClient := kubernetesClient.CoreV1().Services(DitasNamespace)

	logger.Info("Creating or updating VDM service")
	err = CreateOrUpdateResource(logger.WithFields(logrus.Fields{
		"resource": "Service",
		"name":     "vdm",
	}), "vdm", func(name string) (bool, error) {
		existing, err := serviceClient.Get(name, metav1.GetOptions{})
		return err == nil && existing != nil && existing.Name == DitasVDMConfigMapName, err
	},
		serviceClient.Delete,
		func(string) error {
			_, err = serviceClient.Create(&vdmService)
			return err
		})

	if err != nil {
		return err
	}

	logger.Info("VDM successfully deployed")
	/*return ansible.ExecutePlaybook(logger, p.scriptsFolder+"/deploy_vdm.yml", inventoryPath, map[string]string{
		"vars_file":     p.configVariablesPath,
		"config_folder": p.configFolder,
	})*/

	return err
}
