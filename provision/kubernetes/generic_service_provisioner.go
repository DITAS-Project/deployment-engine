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

package kubernetes

import (
	"deployment-engine/model"
	"deployment-engine/utils"
	"errors"
	"fmt"

	"github.com/sirupsen/logrus"
	apiv1 "k8s.io/api/core/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type ServicesConfiguration struct {
	Ports map[string]int
}

type GenericServiceProvisioner struct {
}

func (p GenericServiceProvisioner) ValidateExternalPort(port int, config *KubernetesConfiguration) {

}

func (p GenericServiceProvisioner) Provision(config *KubernetesConfiguration, infra *model.InfrastructureDeploymentInfo, args model.Parameters) error {

	name, ok := args.GetString("name")
	if !ok {
		return errors.New("name parameter is mandatory")
	}

	image, ok := args.GetString("image")
	if !ok {
		return errors.New("image parameter is mandatory")
	}

	internalPort, ok := args.GetInt("internal_port")
	if !ok {
		return errors.New("internal_port parameter is mandatory")
	}

	replicas, ok := args.GetInt("replicas")
	if !ok {
		replicas = 1
	}

	var servicesConfig ServicesConfiguration
	servicesConfigIface, ok := config.DeploymentsConfiguration["services"]
	if ok {
		utils.TransformObject(servicesConfigIface, &servicesConfig)
	} else {
		servicesConfig.Ports = make(map[string]int)
	}

	_, ok = servicesConfig.Ports[name]
	if ok {
		return fmt.Errorf("Service %s already exists", name)
	}

	client, err := NewClient(config.ConfigurationFile)
	if err != nil {
		return err
	}

	logger := logrus.WithFields(logrus.Fields{
		"infrastructure": infra.ID,
		"product":        image,
	})

	terminationPeriod := int64(10)
	labels := map[string]string{
		"serviceName": name,
	}

	externalPort := config.GetNewFreePort()
	servicesConfig.Ports[name] = externalPort
	defer func() {
		if err != nil {
			config.LiberatePort(externalPort)
		}
	}()

	images := ImageSet{
		name: ImageInfo{
			Image:        image,
			InternalPort: internalPort,
			ExternalPort: externalPort,
		},
	}

	var repoSecrets []string
	if config.RegistriesSecret != "" {
		repoSecrets = []string{config.RegistriesSecret}
	}

	pod := GetDeploymentDescription(fmt.Sprintf("%s-deployment", name), int32(replicas), terminationPeriod, labels, images, "", "", repoSecrets)

	_, err = client.CreateOrUpdateDeployment(logger, apiv1.NamespaceDefault, &pod)
	if err != nil {
		return fmt.Errorf("Error creating pod for service %s: %s", name, err.Error())
	}

	service := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeNodePort,
			Selector: labels,
			Ports: []corev1.ServicePort{
				corev1.ServicePort{
					Name:     name,
					Port:     int32(externalPort),
					NodePort: int32(externalPort),
					TargetPort: intstr.IntOrString{
						IntVal: int32(internalPort),
					},
				},
			},
		},
	}

	_, err = client.CreateOrUpdateService(logger, apiv1.NamespaceDefault, &service)
	if err != nil {
		return fmt.Errorf("Error creating service %s: %s", name, err.Error())
	}

	config.DeploymentsConfiguration["services"] = servicesConfig

	return nil
}
