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
	"encoding/json"
	"errors"
	"fmt"

	blueprint "github.com/DITAS-Project/blueprint-go"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	VDCIDProperty            = "vdcId"
	BlueprintProperty        = "blueprint"
	VDMIPProperty            = "vdmIP"
	CAFPortProperty          = "cafPort"
	TombstonePortProperty    = "tombstonePort"
	VariablesProperty        = "variables"
	DalsProperty             = "dals"
	VDCProvisionModeProperty = "mode"
	HostsProperty            = "hosts"

	VDCProvisionModeCreate = "create"
	VDCProvisionModeModify = "modify"

	LoggingAgentPortVariable         = "logging_agent_port"
	LoggingAgentExternalPortVariable = "logging_agent_external_port"

	SLAManagerPortVariable         = "sla_manager_port"
	SLAManagerExternalPortVariable = "sla_manager_external_port"

	//DUEVDCTestPort = 30005
)

type VDCProvisioner struct {
	configFolder   string
	imagesVersions map[string]string
}

func NewVDCProvisioner(configFolder string, imagesVersions map[string]string) *VDCProvisioner {
	return &VDCProvisioner{
		configFolder:   configFolder,
		imagesVersions: imagesVersions,
	}
}

func (p VDCProvisioner) GetImageVersion(imageName string) string {
	version, ok := p.imagesVersions[imageName]
	if !ok {
		return "latest"
	}
	return version
}

func (p VDCProvisioner) CreateVDC(logger *logrus.Entry, config *kubernetes.KubernetesConfiguration, infra *model.InfrastructureDeploymentInfo, args model.Parameters, vdcID string) (model.Parameters, error) {
	result := make(model.Parameters)
	var err error

	blueprintRaw, ok := args[BlueprintProperty]
	if !ok {
		return result, errors.New("Can't find blueprint in parameters")
	}

	bp, ok := blueprintRaw.(blueprint.Blueprint)
	if !ok {
		return result, errors.New("Invalid type for blueprint parameter. Expected blueprint.Blueprint")
	}

	vdmIP, ok := args.GetString(VDMIPProperty)
	if !ok {
		return result, fmt.Errorf("It's necessary to pass the VDM IP in order to deploy VDC")
	}

	varsRaw, ok := args[VariablesProperty]
	if !ok {
		return result, errors.New("Can't find the substitution variables parameter")
	}

	vars, ok := varsRaw.(model.Parameters)
	if !ok {
		return result, errors.New("Invalid type for substitution variables parameter. Expected map[string]interface{}")
	}

	loggingAgentPort, _, err := GetPortPair(vars, LoggingAgentPortVariable, LoggingAgentExternalPortVariable)
	if err != nil {
		return result, err
	}

	slaManagerPort, _, err := GetPortPair(vars, SLAManagerPortVariable, SLAManagerExternalPortVariable)
	if err != nil {
		return result, err
	}

	dalsRaw, ok := args[DalsProperty]
	if !ok {
		return result, errors.New("Can't find DALs information in the parameters")
	}

	dals, ok := dalsRaw.(map[string]string)
	if !ok {
		return result, errors.New("Unexpected type found for DALs information. Expected map[string]string")
	}

	kubeClient, err := kubernetes.NewClient(config.ConfigurationFile)
	if err != nil {
		return result, utils.WrapLogAndReturnError(logger, "Error getting kubernetes client", err)
	}

	if !config.Managed {
		ports, err := kubeClient.GetUsedNodePorts()
		if err != nil {
			return result, utils.WrapLogAndReturnError(logger, "Error getting list of used ports", err)
		}
		config.SetUsedPorts(ports)
	}

	cafExternalPort := config.GetNewFreePort()
	if cafExternalPort < 0 {
		return result, fmt.Errorf("Error reserving port %d: %w", cafExternalPort, err)
	}
	defer func() {
		if err != nil {
			config.LiberatePort(cafExternalPort)
		}
	}()
	result[CAFPortProperty] = cafExternalPort

	tombstonePort := config.GetNewFreePort()
	if tombstonePort < 0 {
		return result, fmt.Errorf("Error reserving port %d: %w", tombstonePort, err)
	}
	defer func() {
		if err != nil {
			config.LiberatePort(tombstonePort)
		}
	}()
	result[TombstonePortProperty] = tombstonePort

	DUEVDCTestPort := config.GetNewFreePort()
	if DUEVDCTestPort < 0 {
		return result, errors.New("Can't find a free port for DUE testing")
	}
	defer func() {
		if err != nil {
			config.LiberatePort(DUEVDCTestPort)
		}
	}()

	logger = logger.WithField("VDC", vdcID)

	var imageSet kubernetes.ImageSet
	utils.TransformObject(bp.InternalStructure.VDCImages, &imageSet)
	imageSet["sla-manager"] = kubernetes.ImageInfo{
		Image:        fmt.Sprintf("ditas/slalite:%s", p.GetImageVersion("slalite")),
		InternalPort: slaManagerPort,
	}
	imageSet["request-monitor"] = kubernetes.ImageInfo{
		Image:        fmt.Sprintf("ditas/vdc-request-monitor:%s", p.GetImageVersion("vdc-request-monitor")),
		InternalPort: 80,
	}
	imageSet["logging-agent"] = kubernetes.ImageInfo{
		Image:        fmt.Sprintf("ditas/vdc-logging-agent:%s", p.GetImageVersion("vdc-logging-agent")),
		InternalPort: loggingAgentPort,
	}
	imageSet["due-vdc"] = kubernetes.ImageInfo{
		Image:        fmt.Sprintf("ditas/due-vdc:%s", p.GetImageVersion("due-vdc")),
		InternalPort: 5000,
	}

	caf, ok := imageSet["caf"]
	if !ok {
		err = errors.New("Can't find CAF image with identifier \"caf\"")
		return result, err
	}
	cafPort := caf.InternalPort

	strBp, err := json.Marshal(bp)
	if err != nil {
		return result, fmt.Errorf("Error marshalling blueprint: %s", err.Error())
	}

	vars["vdcId"] = vdcID
	vars["caf_port"] = cafPort
	vars["infrastructure_id"] = infra.ID

	configMapName := fmt.Sprintf("%s-configmap", vdcID)

	configMap, err := kubernetes.GetConfigMapFromFolder(p.configFolder+"/vdcs", configMapName, vars)
	if err != nil {
		logger.WithError(err).Error("Error reading configuration map")
		return result, err
	}

	configMap.Data["blueprint.json"] = string(strBp)

	logger.Info("Creating or updating VDC config map")
	_, err = kubeClient.CreateOrUpdateConfigMap(logger, DitasNamespace, &configMap)

	if err != nil {
		return result, err
	}

	vdcLabels := map[string]string{
		"component": vdcID,
	}

	var repoSecrets []string
	if config.RegistriesSecret != "" {
		repoSecrets = []string{config.RegistriesSecret}
	}

	hostAlias := make([]corev1.HostAlias, 0, len(bp.InternalStructure.DALImages)+1)
	for dalName, dalInfo := range bp.InternalStructure.DALImages {

		var dalIP string
		if ip, ok := dals[dalName]; ok {
			dalIP = ip
		} else {
			dalIP = dalInfo.OriginalIP
			if customIP, ok := dalInfo.ClusterOriginalIPs[infra.Name]; ok {
				dalIP = customIP
			}
		}

		if dalIP != "" {
			hostAlias = append(hostAlias, corev1.HostAlias{
				IP:        dalIP,
				Hostnames: []string{dalName},
			})
		}

		for imageName, imageInfo := range dalInfo.Images {
			if imageInfo.ExternalPort != nil {
				varName := fmt.Sprintf("dal.%s.%s.port", dalName, imageName)
				vars[varName] = fmt.Sprintf("%d", *imageInfo.ExternalPort)
			}
		}
	}

	if vdmIP != "" {
		hostAlias = append(hostAlias, corev1.HostAlias{
			IP:        vdmIP,
			Hostnames: []string{"vdm"},
		})
	}

	kubernetes.ReplaceEnvVars(imageSet, vars)

	vdcDeployment := kubernetes.GetDeploymentDescription(vdcID, int32(1), int64(30), vdcLabels, imageSet, configMapName, "/etc/ditas", repoSecrets, nil)

	if len(hostAlias) > 0 {
		vdcDeployment.Spec.Template.Spec.HostAliases = hostAlias
	}

	shareNamespace := true
	vdcDeployment.Spec.Template.Spec.ShareProcessNamespace = &shareNamespace

	logger.Info("Creating or updating VDC pod")
	_, err = kubeClient.CreateOrUpdateDeployment(logger, DitasNamespace, &vdcDeployment)

	if err != nil {
		return result, err
	}

	/*ports := make([]corev1.ServicePort, 0)

	for _, image := range imageSet {
		if image.ExternalPort != 0 {
			ports = append(ports, corev1.ServicePort{
				Port:       int32(image.ExternalPort),
				NodePort:   int32(image.ExternalPort),
				TargetPort: intstr.FromInt(image.InternalPort),
			})
		}
	}*/

	ports := []corev1.ServicePort{
		corev1.ServicePort{
			Port:       int32(cafExternalPort),
			NodePort:   int32(cafExternalPort),
			TargetPort: intstr.FromInt(80),
			Name:       "caf",
		},
		corev1.ServicePort{
			Port:       int32(tombstonePort),
			NodePort:   int32(tombstonePort),
			TargetPort: intstr.FromInt(3000),
			Name:       "tombstone",
		},
		corev1.ServicePort{
			Port:       int32(DUEVDCTestPort),
			NodePort:   int32(DUEVDCTestPort),
			TargetPort: intstr.FromInt(5000),
			Name:       "due",
		},
	}

	vdcService := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: vdcID,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeNodePort,
			Selector: vdcLabels,
			Ports:    ports,
		},
	}

	logger.Info("Creating or updating VDC service")
	_, err = kubeClient.CreateOrUpdateService(logger, DitasNamespace, &vdcService)
	if err != nil {
		return result, err
	}

	logger.Info("VDC successfully deployed")

	return result, err
}

func (p VDCProvisioner) ModifyVDC(logger *logrus.Entry, config *kubernetes.KubernetesConfiguration, infra *model.InfrastructureDeploymentInfo, args model.Parameters, vdcID string) (model.Parameters, error) {
	result := make(model.Parameters)

	hostsRaw, ok := args[HostsProperty]
	if !ok {
		return result, errors.New("Can't find list of hosts to modify")
	}

	hosts, ok := hostsRaw.(map[string]string)
	if !ok {
		return result, errors.New("Unexpected type found for list of hosts to modidy. Expected map[string]string")
	}

	kubeClient, err := kubernetes.NewClient(config.ConfigurationFile)
	if err != nil {
		return result, utils.WrapLogAndReturnError(logger, "Error getting kubernetes client", err)
	}

	vdcDeployment, err := kubeClient.Client.AppsV1().Deployments(DitasNamespace).Get(vdcID, metav1.GetOptions{})
	if err != nil {
		return result, fmt.Errorf("Error getting deployment for VDC %s: %w", vdcID, err)
	}

	for i, alias := range vdcDeployment.Spec.Template.Spec.HostAliases {
		for _, hostname := range alias.Hostnames {
			if newIP, ok := hosts[hostname]; ok {
				alias.IP = newIP
				vdcDeployment.Spec.Template.Spec.HostAliases[i] = alias
			}
		}
	}

	_, err = kubeClient.Client.AppsV1().Deployments(DitasNamespace).Update(vdcDeployment)
	if err != nil {
		return result, fmt.Errorf("Error updating deployment %s", vdcID)
	}

	return result, nil
}

func (p VDCProvisioner) Provision(config *kubernetes.KubernetesConfiguration, infra *model.InfrastructureDeploymentInfo, args model.Parameters) (model.Parameters, error) {
	result := make(model.Parameters)
	mode, ok := args.GetString(VDCProvisionModeProperty)
	if !ok {
		return result, errors.New("Operation mode for VDC provisioner is needed")
	}

	vdcID, ok := args.GetString(VDCIDProperty)
	if !ok {
		return result, errors.New("Can't find VDC identifier in parameters")
	}

	logger := logrus.WithFields(logrus.Fields{
		"vdc":            vdcID,
		"infrastructure": infra.ID,
	})

	switch mode {
	case VDCProvisionModeCreate:
		return p.CreateVDC(logger, config, infra, args, vdcID)
	case VDCProvisionModeModify:
		return p.ModifyVDC(logger, config, infra, args, vdcID)
	default:
		return result, fmt.Errorf("Unrecognized operation mode: %s", mode)
	}

}
