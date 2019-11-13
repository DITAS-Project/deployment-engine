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
	DitasNamespace                    = "default"
	DitasVDMConfigMapName             = "vdm"
	BlueprintIDProperty               = "blueprintId"
	CMEPortVariable                   = "cme_port"
	CMEExternalPortVariable           = "cme_external_port"
	DataAnalyticsPortVariable         = "data_analytics_port"
	DataAnalyticsExternalPortVariable = "data_analytics_external_port"
	DS4MPortVariable                  = "ds4m_port"
	DS4MExternalPortVariable          = "ds4m_external_port"
	DUEVDMPortVariable                = "due_vdm_port"
	DUEVDMExternalPortVariable        = "due_vdm_external_port"

	DS4MDefaultExternalPort          = 30003
	DataAnalyticsDefaultExternalPort = 30006
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

func (p VDMProvisioner) GetImageVersion(imageName string) string {
	version, ok := p.imagesVersions[imageName]
	if !ok {
		return "latest"
	}
	return version
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

	vars, ok := varsRaw.(model.Parameters)
	if !ok {
		return result, errors.New("Invalid type for substitution variables parameter. Expected map[string]interface{}")
	}

	blueprintID, ok := args.GetString(BlueprintIDProperty)
	if !ok {
		return result, errors.New("Blueprint identifier parameter is mandatory to deploy a VDM")
	}
	vars["blueprint_id"] = blueprintID

	cmePort, cmeExternalPort, err := GetPortPair(vars, CMEPortVariable, CMEExternalPortVariable)
	if err != nil {
		return result, err
	}

	duePort, dueExternalPort, err := GetPortPair(vars, DUEVDMPortVariable, DUEVDMExternalPortVariable)
	if err != nil {
		return result, err
	}

	ds4mPort, ds4mExternalPort, err := GetPortPair(vars, DS4MPortVariable, DS4MExternalPortVariable)
	if err != nil {
		return result, err
	}
	if ds4mExternalPort == 0 {
		ds4mExternalPort = DS4MDefaultExternalPort
	}

	daPort, daExternalPort, err := GetPortPair(vars, DataAnalyticsPortVariable, DataAnalyticsExternalPortVariable)
	if err != nil {
		return result, err
	}
	if daExternalPort == 0 {
		daExternalPort = DataAnalyticsDefaultExternalPort
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
		Image:        fmt.Sprintf("ditas/decision-system-for-data-and-computation-movement:%s", p.GetImageVersion("ds4m")),
		InternalPort: ds4mPort,
	}
	imageSet["cme"] = kubernetes.ImageInfo{
		Image:        fmt.Sprintf("ditas/computation-movement-enactor:%s", p.GetImageVersion("cme")),
		InternalPort: cmePort,
	}
	imageSet["data-analytics"] = kubernetes.ImageInfo{
		Image:        fmt.Sprintf("ditas/data_analytics:%s", p.GetImageVersion("data-analytics")),
		InternalPort: daPort,
	}
	imageSet["due"] = kubernetes.ImageInfo{
		Image:        fmt.Sprintf("ditas/due-vdm:%s", p.GetImageVersion("due-vdm")),
		InternalPort: duePort,
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

	servicePorts := []corev1.ServicePort{
		corev1.ServicePort{
			Name:       "ds4m",
			NodePort:   int32(ds4mExternalPort),
			Port:       int32(ds4mExternalPort),
			TargetPort: intstr.FromInt(ds4mPort),
		},
		corev1.ServicePort{
			Name:       "data-analytics",
			NodePort:   int32(daExternalPort),
			Port:       int32(daExternalPort),
			TargetPort: intstr.FromInt(daExternalPort),
		},
	}

	servicePorts = AppendDebugPort(servicePorts, "cme", cmePort, cmeExternalPort)
	servicePorts = AppendDebugPort(servicePorts, "due", duePort, dueExternalPort)

	vdmService := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "vdm",
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeNodePort,
			Selector: vdmLabels,
			Ports:    servicePorts,
		},
	}

	err = config.ClaimPort(ds4mExternalPort)
	if err != nil {
		return result, utils.WrapLogAndReturnError(logger, "Error reserving DS4M port", err)
	}

	err = config.ClaimPort(daExternalPort)
	if err != nil {
		config.LiberatePort(ds4mExternalPort)
		return result, utils.WrapLogAndReturnError(logger, "Error reserving Data Analytics port", err)
	}

	if cmeExternalPort != 0 {
		err = config.ClaimPort(cmeExternalPort)
		if err != nil {
			config.LiberatePort(ds4mExternalPort)
			config.LiberatePort(daExternalPort)
			return result, utils.WrapLogAndReturnError(logger, "Error reserving Data Analytics port", err)
		}
	}

	if dueExternalPort != 0 {
		err = config.ClaimPort(dueExternalPort)
		if err != nil {
			config.LiberatePort(ds4mExternalPort)
			config.LiberatePort(daExternalPort)
			if cmeExternalPort != 0 {
				config.LiberatePort(cmeExternalPort)
			}
			return result, utils.WrapLogAndReturnError(logger, "Error reserving Data Analytics port", err)
		}
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
