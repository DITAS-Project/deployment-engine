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
	"errors"
	"fmt"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	DitasNamespace        = "default"
	DitasVDMConfigMapName = "vdm"
	BlueprintIDProperty   = "blueprintId"

	CMEExternalPort = 30090
)

type VDMProvisioner struct {
	scriptsFolder       string
	configVariablesPath string
	configFolder        string
	imagesVersions      map[string]string
}

func NewVDMProvisioner(scriptsFolder, configVariablesPath, configFolder string, imagesVersions map[string]string) VDMProvisioner {
	return VDMProvisioner{
		scriptsFolder:       scriptsFolder,
		configVariablesPath: configVariablesPath,
		configFolder:        configFolder,
		imagesVersions:      imagesVersions,
	}
}

func (p VDMProvisioner) Provision(config *kubernetes.KubernetesConfiguration, infra *model.InfrastructureDeploymentInfo, args model.Parameters) (model.Parameters, error) {

	result := make(model.Parameters)
	logger := logrus.WithFields(logrus.Fields{
		"infrastructure": infra.ID,
	})

	varsRaw, ok := args[VariablesProperty]
	if !ok {
		return result, errors.New("Can't find the substitution variables parameter")
	}

	vars, ok := varsRaw.(map[string]interface{})
	if !ok {
		return result, errors.New("Invalid type for substitution variables parameter. Expected map[string]interface{}")
	}

	blueprintID, ok := args.GetString(BlueprintIDProperty)
	if !ok {
		return result, errors.New("Blueprint identifier parameter is mandatory to deploy a VDM")
	}
	vars["blueprint_id"] = blueprintID

	cmePortRaw, ok := vars["cme_port"]
	if !ok {
		return result, errors.New("cme_port variable is mandatory in configuration")
	}
	cmePort, ok := cmePortRaw.(int)
	if !ok {
		return result, errors.New("Invalid type for variable cme_port. Expected int")
	}

	configMap, err := kubernetes.GetConfigMapFromFolder(p.configFolder+"/vdm", DitasVDMConfigMapName, vars)
	if err != nil {
		return result, utils.WrapLogAndReturnError(logger, "Error reading configuration map", err)
	}

	kubeClient, err := kubernetes.NewClient(config.ConfigurationFile)
	if err != nil {
		logger.WithError(err).Error("Error getting kubernetes client")
		return result, utils.WrapLogAndReturnError(logger, "Error getting kubernetes client", err)
	}

	if !config.Managed {
		ports, err := kubeClient.GetUsedNodePorts()
		if err != nil {
			return result, utils.WrapLogAndReturnError(logger, "Error getting list of used ports", err)
		}
		config.SetUsedPorts(ports)
	}

	logger.Info("Creating or updating VDM config map")
	_, err = kubeClient.CreateOrUpdateConfigMap(logger, DitasNamespace, &configMap)

	if err != nil {
		return result, err
	}

	vdmLabels := map[string]string{
		"component": "vdm",
	}

	imageSet := make(kubernetes.ImageSet)
	imageSet["ds4m"] = kubernetes.ImageInfo{
		Image:        "ditas/decision-system-for-data-and-computation-movement",
		InternalPort: 8080,
	}
	imageSet["cme"] = kubernetes.ImageInfo{
		Image:        "ditas/computation-movement-enactor",
		InternalPort: cmePort,
	}

	var repSecrets []string
	if config.RegistriesSecret != "" {
		repSecrets = []string{config.RegistriesSecret}
	}

	vdmPVC := kubernetes.VolumeData{
		Name:         "ds4m",
		MountPoint:   "/var/ditas/vdm",
		PVCName:      "ds4m",
		StorageClass: "rook-ceph-block-single",
		Size:         "100Mi",
	}

	pvc, err := kubernetes.GetPersistentVolumeClaim(vdmPVC)
	if err != nil {
		return result, fmt.Errorf("Error getting PVC description for DS4M: %w", err)
	}

	logger.Infof("Creating PVC %s", pvc.Name)
	_, err = kubeClient.CreateOrUpdatePVC(logger, DitasNamespace, &pvc)
	if err != nil {
		return result, fmt.Errorf("Error creating PVC %s: %w", pvc.Name, err)
	}

	vdmDeployment := kubernetes.GetDeploymentDescription("vdm", int32(1), int64(30), vdmLabels, imageSet, DitasVDMConfigMapName, "/etc/ditas", repSecrets, []kubernetes.VolumeData{vdmPVC})

	logger.Info("Creating or updating VDM pod")
	_, err = kubeClient.CreateOrUpdateDeployment(logger, DitasNamespace, &vdmDeployment)

	if err != nil {
		return result, err
	}

	vdmService := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "vdm",
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeNodePort,
			Selector: vdmLabels,
			Ports: []corev1.ServicePort{
				corev1.ServicePort{
					Name:       "cme",
					NodePort:   CMEExternalPort,
					Port:       CMEExternalPort,
					TargetPort: intstr.FromInt(cmePort),
				},
			},
		},
	}

	err = config.ClaimPort(CMEExternalPort)
	if err != nil {
		config.LiberatePort(CMEExternalPort)
		return result, utils.WrapLogAndReturnError(logger, "Error reserving CME port", err)
	}

	logger.Info("Creating or updating VDM service")
	_, err = kubeClient.CreateOrUpdateService(logger, DitasNamespace, &vdmService)
	if err != nil {
		return result, err
	}

	config.DeploymentsConfiguration["VDM"] = true

	logger.Info("VDM successfully deployed")

	return result, err
}
