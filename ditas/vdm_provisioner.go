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
	"deployment-engine/provision/kubernetes"
	"deployment-engine/utils"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	DitasNamespace        = "default"
	DitasVDMConfigMapName = "vdm"
)

type VDMProvisioner struct {
	scriptsFolder       string
	configVariablesPath string
	configFolder        string
}

func NewVDMProvisioner(scriptsFolder, configVariablesPath, configFolder string) VDMProvisioner {
	return VDMProvisioner{
		scriptsFolder:       scriptsFolder,
		configVariablesPath: configVariablesPath,
		configFolder:        configFolder,
	}
}

func (p VDMProvisioner) Provision(config *kubernetes.KubernetesConfiguration, deploymentID string, infra *model.InfrastructureDeploymentInfo, args model.Parameters) error {

	logger := logrus.WithFields(logrus.Fields{
		"deployment":     deploymentID,
		"infrastructure": infra.ID,
	})

	vars, err := utils.GetVarsFromConfigFolder()
	if err != nil {
		return err
	}

	configMap, err := kubernetes.GetConfigMapFromFolder(p.configFolder+"/vdm", DitasVDMConfigMapName, vars)
	if err != nil {
		logger.WithError(err).Error("Error reading configuration map")
		return err
	}

	kubeClient, err := kubernetes.NewClient(config.ConfigurationFile)
	if err != nil {
		logger.WithError(err).Error("Error getting kubernetes client")
		return err
	}

	logger.Info("Creating or updating VDM config map")
	_, err = kubeClient.CreateOrUpdateConfigMap(logger, DitasNamespace, &configMap)

	if err != nil {
		return err
	}

	vdmLabels := map[string]string{
		"component": "vdm",
	}

	imageSet := make(kubernetes.ImageSet)
	imageSet["ds4m"] = kubernetes.ImageInfo{
		Image:        "ditas/decision-system-for-data-and-computation-movement",
		InternalPort: 8080,
	}

	var repSecrets []string
	if config.RegistriesSecret != "" {
		repSecrets = []string{config.RegistriesSecret}
	}

	vdmDeployment := kubernetes.GetDeploymentDescription("vdm", int32(1), int64(30), vdmLabels, imageSet, DitasVDMConfigMapName, "/etc/ditas", repSecrets)

	logger.Info("Creating or updating VDM pod")
	_, err = kubeClient.CreateOrUpdateDeployment(logger, DitasNamespace, &vdmDeployment)

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

	logger.Info("Creating or updating VDM service")
	_, err = kubeClient.CreateOrUpdateService(logger, DitasNamespace, &vdmService)
	if err != nil {
		return err
	}

	config.DeploymentsConfiguration["VDM"] = true

	logger.Info("VDM successfully deployed")

	return err
}
