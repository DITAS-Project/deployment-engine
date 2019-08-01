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

func (p VDCProvisioner) transformDeploymentInfo(src model.DeploymentInfo) (blueprint.DeploymentInfo, error) {
	var target blueprint.DeploymentInfo
	err := utils.TransformObject(src, &target)
	if err != nil {
		return target, err
	}

	return target, err
}

func (p VDCProvisioner) Provision(config *kubernetes.KubernetesConfiguration, deploymentID string, infra *model.InfrastructureDeploymentInfo, args model.Parameters) error {

	var err error
	logger := logrus.WithFields(logrus.Fields{
		"deployment":     deploymentID,
		"infrastructure": infra.ID,
	})

	blueprintRaw, ok := args["blueprint"]
	if !ok {
		return errors.New("Can't find blueprint in parameters")
	}

	bp := blueprintRaw.(blueprint.Blueprint)

	deploymentRaw, ok := args["deployment"]
	if !ok {
		return errors.New("Can't find deployment in parameters")
	}

	deployment := deploymentRaw.(model.DeploymentInfo)

	var vdcsConfig VDCsConfiguration
	ok, err = utils.GetObjectFromMap(config.DeploymentsConfiguration, "VDC", &vdcsConfig)
	if err != nil {
		return fmt.Errorf("Error getting VDCs configuration for infrastructure %s: %s", infra.ID, err.Error())
	}
	if !ok {
		vdcsConfig = VDCsConfiguration{
			NumVDCs: 0,
			VDCs:    make(map[string]VDCConfiguration),
		}
		config.DeploymentsConfiguration["VDC"] = vdcsConfig
	}

	tombstonePort, ok := args.GetInt("tombstonePort")
	if !ok {
		return errors.New("Tombstone port is mandatory. Please check the configuration file for ditas.tombstone.port property")
	}

	vdcID, ok := args.GetString("vdcId")
	if !ok {
		return errors.New("Can't find VDC identifier in parameters")
	}

	isMove, ok := args.GetBool("move")
	if !ok {
		isMove = false
	}

	vdmIP, ok := args.GetString("vdmIP")
	if isMove && !ok {
		return fmt.Errorf("It's necessary to pass the VDM IP in order to move a VDC")
	}

	vdcConfig, ok := vdcsConfig.VDCs[vdcID]
	if ok {
		return fmt.Errorf("VDC %s already deployed", vdcID)
	}

	vdcConfig = VDCConfiguration{
		Ports: make(map[string]int),
	}

	logger = logger.WithField("VDC", vdcID)

	var imageSet kubernetes.ImageSet
	utils.TransformObject(bp.InternalStructure.VDCImages, &imageSet)
	imageSet["sla-manager"] = kubernetes.ImageInfo{
		Image: "ditas/slalite",
	}
	imageSet["request-monitor"] = kubernetes.ImageInfo{
		Image:        "ditas/vdc-request-monitor:production",
		InternalPort: 80,
	}
	imageSet["logging-agent"] = kubernetes.ImageInfo{
		Image:        "ditas/vdc-logging-agent:production",
		InternalPort: 8484,
	}

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

	vdcsConfig.NumVDCs++

	bpDeployment, err := p.transformDeploymentInfo(deployment)
	if err != nil {
		return fmt.Errorf("Error transforming deployment to write the COOKBOOK_APPENDIX section: %s", err.Error())
	}
	bp.CookbookAppendix.Deployment = bpDeployment

	strBp, err := json.Marshal(bp)
	if err != nil {
		return fmt.Errorf("Error marshalling blueprint: %s", err.Error())
	}
	vdcConfig.Blueprint = string(strBp)

	vdcsConfig.VDCs[vdcID] = vdcConfig

	vars, err := utils.GetVarsFromConfigFolder()
	if err != nil {
		return err
	}
	vars["vdcId"] = vdcID

	caf, ok := imageSet["caf"]
	if !ok {
		return errors.New("Can't find CAF image with identifier \"caf\"")
	}
	cafPort := caf.InternalPort
	cafExternalPort := caf.ExternalPort
	vars["caf_port"] = cafPort

	configMapName := fmt.Sprintf("%s-configmap", vdcID)

	configMap, err := kubernetes.GetConfigMapFromFolder(p.configFolder+"/vdcs", configMapName, vars)
	if err != nil {
		logger.WithError(err).Error("Error reading configuration map")
		return err
	}

	configMap.Data["blueprint.json"] = string(strBp)

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
		"component": vdcID,
	}

	var repoSecrets []string
	if config.RegistriesSecret != "" {
		repoSecrets = []string{config.RegistriesSecret}
	}

	vdcDeployment := kubernetes.GetDeploymentDescription(vdcID, int32(1), int64(30), vdcLabels, imageSet, configMapName, "/etc/ditas", repoSecrets)

	if isMove {
		hostAlias := []corev1.HostAlias{
			corev1.HostAlias{
				IP:        vdmIP,
				Hostnames: []string{"vdm"},
			},
		}
		vdcDeployment.Spec.Template.Spec.HostAliases = hostAlias
	}
	shareNamespace := true
	vdcDeployment.Spec.Template.Spec.ShareProcessNamespace = &shareNamespace

	logger.Info("Creating or updating VDC pod")
	_, err = kubeClient.CreateOrUpdateDeployment(logger, DitasNamespace, &vdcDeployment)

	if err != nil {
		return err
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
		return err
	}

	config.DeploymentsConfiguration["VDC"] = vdcsConfig

	logger.Info("VDC successfully deployed")

	return err
}
