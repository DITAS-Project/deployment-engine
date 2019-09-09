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
	"strconv"
	"strings"

	blueprint "github.com/DITAS-Project/blueprint-go"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type VDCProvisioner struct {
	configFolder string
}

func NewVDCProvisioner(configFolder string) *VDCProvisioner {
	return &VDCProvisioner{
		configFolder: configFolder,
	}
}

// FillEnvVars runs over the VDC images trying to find environment variables whose value is specified as ${var_name} and expand it with the corresponding value
// Variables supported:
// - dal.<dal_name>.name: Hostname of a DAL that will resolve directly to the DAL IP
// - dal.<dal_name>.<dal_image_name>.port: External port of an image in a DAL
func (p VDCProvisioner) FillEnvVars(dals map[string]blueprint.DALImage, vdcImages kubernetes.ImageSet) (kubernetes.ImageSet, error) {
	for imageName, vdcImage := range vdcImages {
		if vdcImage.Environment != nil {
			for variable, value := range vdcImage.Environment {
				if strings.HasPrefix(value, "${") && strings.HasSuffix(value, "}") {
					// ENV var value is a variable. Let's strip the ${} surrounding
					varName := value[2 : len(value)-1]
					varPath := strings.Split(varName, ".")
					if len(varPath) < 3 {
						return vdcImages, fmt.Errorf("Invalid variable name %s found in environment of VDC image %s", varName, imageName)
					}
					if varPath[0] == "dal" {
						// The first part refers to a dal. Let's find its information
						dalName := varPath[1]
						dalInfo, ok := dals[dalName]
						if !ok {
							return vdcImages, fmt.Errorf("Can't find DAL with name %s to expand variable %s in VDC image %s", dalName, varName, imageName)
						}
						var finalValue string
						if len(varPath) == 3 {
							// Length of 3 and it starts with dal. Only dal.<dal_name>.name is supported
							if varPath[2] == "name" {
								finalValue = dalName
							} else {
								return vdcImages, fmt.Errorf("Unrecognized variable %s found in VDC image %s", varName, imageName)
							}
						} else {
							// Length > 3. It has to be dal.<dal_name>.<image_name>.port or error
							dalImageName := varPath[2]
							dalImageInfo, ok := dalInfo.Images[dalImageName]
							if !ok {
								return vdcImages, fmt.Errorf("Can't find image %s in DAL %s information to expand variable %s of VDC image %s", dalImageName, dalName, varName, imageName)
							}
							if varPath[3] == "port" {
								if dalImageInfo.ExternalPort == nil {
									return vdcImages, fmt.Errorf("Empty external port found in image %s of DAL %s when trying to expand variable %s of VDC image %s", dalImageName, dalName, varName, imageName)
								}
								finalValue = strconv.FormatInt(*dalImageInfo.ExternalPort, 10)
							}
						}
						if finalValue != "" {
							vdcImage.Environment[variable] = finalValue
						}
					}
				}
			}
			vdcImages[imageName] = vdcImage
		}
	}
	return vdcImages, nil
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

	tombstonePort, ok := args.GetInt("tombstonePort")
	if !ok {
		return errors.New("Tombstone port is mandatory")
	}
	err = config.ClaimPort(tombstonePort)
	if err != nil {
		return fmt.Errorf("Error reserving port %d: %w", tombstonePort, err)
	}
	defer func() {
		if err != nil {
			config.LiberatePort(tombstonePort)
		}
	}()

	vdcID, ok := args.GetString("vdcId")
	if !ok {
		return errors.New("Can't find VDC identifier in parameters")
	}

	vdmIP, ok := args.GetString("vdmIP")
	if !ok {
		return fmt.Errorf("It's necessary to pass the VDM IP in order to deploy VDC")
	}

	logger = logger.WithField("VDC", vdcID)

	dals := bp.InternalStructure.DALImages

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

	imageSet, err = p.FillEnvVars(dals, imageSet)
	if err != nil {
		return err
	}

	caf, ok := imageSet["caf"]
	if !ok {
		return errors.New("Can't find CAF image with identifier \"caf\"")
	}
	cafPort := caf.InternalPort
	cafExternalPort := caf.ExternalPort

	err = config.ClaimPort(cafExternalPort)
	if err != nil {
		return fmt.Errorf("Error reserving port %d: %s", cafExternalPort, err.Error())
	}
	defer func() {
		if err != nil {
			config.LiberatePort(cafExternalPort)
		}
	}()

	strBp, err := json.Marshal(bp)
	if err != nil {
		return fmt.Errorf("Error marshalling blueprint: %s", err.Error())
	}

	vars, err := utils.GetVarsFromConfigFolder()
	if err != nil {
		return err
	}
	vars["vdcId"] = vdcID
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

	if vdmIP != "" {
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

	logger.Info("VDC successfully deployed")

	return err
}
