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
	"fmt"

	blueprint "github.com/DITAS-Project/blueprint-go"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type VDCConfiguration struct {
	Ports     map[string]int
	Blueprint string
}

type VDCsConfiguration struct {
	NumVDCs int
	VDCs    map[string]VDCConfiguration
}

type VDCProvisioner struct {
	configFolder string
}

func NewVDCProvisioner(configFolder string) *VDCProvisioner {
	return &VDCProvisioner{
		configFolder: configFolder,
	}
}

func (p VDCProvisioner) FreePorts(config *kubernetes.KubernetesConfiguration, vdcConfig VDCConfiguration, err error) {
	if err != nil {
		for _, port := range vdcConfig.Ports {
			config.LiberatePort(port)
		}
	}
}

func (p VDCProvisioner) Provision(config *kubernetes.KubernetesConfiguration, deploymentID string, infra *model.InfrastructureDeploymentInfo, args model.Parameters) error {

	logger := logrus.WithFields(logrus.Fields{
		"deployment":     deploymentID,
		"infrastructure": infra.ID,
	})

	blueprintRaw, ok := args["blueprint"]
	if !ok {
		return fmt.Errorf("Can't find blueprint in parameters")
	}

	bp := blueprintRaw.(blueprint.BlueprintType)

	var vdcsConfig VDCsConfiguration
	vdcConfigRaw, ok := config.DeploymentsConfiguration["VDC"]
	if !ok {
		vdcsConfig = VDCsConfiguration{
			NumVDCs: 0,
			VDCs:    make(map[string]VDCConfiguration),
		}
		config.DeploymentsConfiguration["VDC"] = vdcsConfig
	} else {
		err := utils.TransformObject(vdcConfigRaw, &vdcsConfig)
		if err != nil {
			return fmt.Errorf("Error reading VDCs configuration: %s\n", err.Error())
		}
	}

	vdcId, ok := args.GetString("vdcId")
	if !ok {
		vdcId = fmt.Sprintf("vdc-%d", vdcsConfig.NumVDCs)
	}
	isMove := ok

	vdmIP, ok := args.GetString("vdmIP")
	if isMove && !ok {
		return fmt.Errorf("It's necessary to pass the VDM IP in order to move a VDC")
	}

	vdcConfig, ok := vdcsConfig.VDCs[vdcId]
	if ok {
		return fmt.Errorf("VDC %s already deployed", vdcId)
	}
	vdcConfig = VDCConfiguration{
		Ports: make(map[string]int),
	}

	logger = logger.WithField("VDC", vdcId)

	var imageSet kubernetes.ImageSet
	utils.TransformObject(bp.InternalStructure.VDCImages, &imageSet)
	imageSet["sla-manager"] = kubernetes.ImageInfo{
		Image: "ditas/slalite",
	}

	var err error
	defer func() {
		p.FreePorts(config, vdcConfig, err)
	}()
	for _, image := range imageSet {
		if image.ExternalPort != 0 {
			err = config.ClaimPort(image.ExternalPort)
			if err != nil {
				return fmt.Errorf("Can't claim port %d: %s\n", image.ExternalPort, err.Error())
			}
			vdcConfig.Ports[image.Image] = image.ExternalPort
		}
	}

	vars, err := utils.GetVarsFromConfigFolder()
	if err != nil {
		return err
	}
	vars["vdcId"] = vdcId

	configMapName := fmt.Sprintf("%s-configmap", vdcId)

	configMap, err := kubernetes.GetConfigMapFromFolder(p.configFolder+"/vdcs", configMapName, vars)
	if err != nil {
		logger.WithError(err).Error("Error reading configuration map")
		return err
	}

	strBp, err := json.Marshal(bp)
	if err != nil {
		return fmt.Errorf("Error marshalling blueprint: %s\n", err.Error())
	}

	configMap.Data["blueprint.json"] = string(strBp)
	vdcConfig.Blueprint = configMap.Data["blueprint.json"]

	kubeClient, err := kubernetes.NewClient(config.ConfigurationFile)
	if err != nil {
		logger.WithError(err).Error("Error getting kubernetes client")
		return err
	}

	logger.Info("Creating or updating VDC config map")
	_, err = kubeClient.CreateOrUpdateConfigMap(logger, DitasNamespace, &configMap)

	if err != nil {
		return err
	}

	vdcLabels := map[string]string{
		"component": vdcId,
	}

	var repoSecrets []string
	if config.RegistriesSecret != "" {
		repoSecrets = []string{config.RegistriesSecret}
	}

	vdcDeployment := kubernetes.GetDeploymentDescription(vdcId, int32(1), int64(30), vdcLabels, imageSet, configMapName, "/etc/ditas", repoSecrets)

	if isMove {
		hostAlias := []corev1.HostAlias{
			corev1.HostAlias{
				IP:        vdmIP,
				Hostnames: []string{"vdm"},
			},
		}
		vdcDeployment.Spec.Template.Spec.HostAliases = hostAlias
	}

	logger.Info("Creating or updating VDC pod")
	_, err = kubeClient.CreateOrUpdateDeployment(logger, DitasNamespace, &vdcDeployment)

	if err != nil {
		return err
	}

	ports := make([]corev1.ServicePort, 0)

	for _, image := range imageSet {
		if image.ExternalPort != 0 {
			ports = append(ports, corev1.ServicePort{
				Port:       int32(image.ExternalPort),
				NodePort:   int32(image.ExternalPort),
				TargetPort: intstr.FromInt(image.InternalPort),
			})
		}
	}

	vdcService := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: vdcId,
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
		return err
	}

	vdcsConfig.NumVDCs++
	vdcsConfig.VDCs[vdcId] = vdcConfig
	config.DeploymentsConfiguration["VDC"] = vdcsConfig

	logger.Info("VDC successfully deployed")

	return err
}
