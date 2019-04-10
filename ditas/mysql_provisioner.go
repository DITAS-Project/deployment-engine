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
	"context"
	"deployment-engine/model"
	"deployment-engine/provision/ansible"
	"errors"
	"fmt"
	"strconv"

	"go.mongodb.org/mongo-driver/bson"

	"go.mongodb.org/mongo-driver/mongo"

	blueprint "github.com/DITAS-Project/blueprint-go"
	"github.com/sethvargo/go-password/password"
	"github.com/sirupsen/logrus"
	apiv1 "k8s.io/api/core/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type MySQLProvisioner struct {
	parent        *ansible.Provisioner
	scriptsFolder string
	collection    *mongo.Collection
}

func NewMySQLProvisioner(parent *ansible.Provisioner, scriptsFolder string, collection *mongo.Collection) MySQLProvisioner {
	return MySQLProvisioner{
		parent:        parent,
		scriptsFolder: scriptsFolder,
		collection:    collection,
	}
}

func (p MySQLProvisioner) BuildInventory(deploymentID string, infra model.InfrastructureDeploymentInfo, args map[string][]string) (ansible.Inventory, error) {
	return p.parent.Provisioners["kubeadm"].BuildInventory(deploymentID, infra, args)
}

func (p MySQLProvisioner) DeployProduct(inventoryPath, deploymentID string, infra model.InfrastructureDeploymentInfo, args map[string][]string) error {

	logger := logrus.WithFields(logrus.Fields{
		"deployment":     deploymentID,
		"infrastructure": infra.ID,
	})

	sizes, ok := args["size"]
	if !ok || sizes == nil || len(sizes) == 0 {
		_, err := resource.ParseQuantity(sizes[0])
		if err != nil {
			return fmt.Errorf("Storage size %s is invalid: %s", sizes[0], err.Error())
		}
		return errors.New("Storage size is mandatory for this datasource")
	}

	has, ok := args["ha"]
	ha := false
	if ok && has != nil && len(has) > 0 {
		ha, _ = strconv.ParseBool(has[0])
	}

	var depInfo VDCInformation
	err := p.collection.FindOne(context.Background(), bson.M{"deployment_id": deploymentID}).Decode(&depInfo)
	if err != nil {
		return err
	}

	infraInformation, ok := depInfo.InfraVDCs[infra.ID]
	if !ok {
		infraInformation = initializeVDCInformation()
	}

	mysqlDatasources, ok := infraInformation.Datasources["mysql"]
	if !ok {
		mysqlDatasources = make(map[string]int)
	}

	dsId := fmt.Sprintf("mysql%d", len(mysqlDatasources))
	secretId := dsId + "pw"
	volumeId := dsId + "data"
	servicePort := infraInformation.LastDatasourcePort
	password, err := password.Generate(10, 3, 2, false, false)

	if err != nil {
		return err
	}

	storageclass := "rook-ceph-block-single"
	if ha {
		storageclass = "rook-ceph-block-ha"
	}

	secretData := SecretData{
		SecretId: secretId,
		EnvVars: map[string]string{
			"MYSQL_ROOT_PASSWORD": "password",
		},
		Data: map[string]string{
			"password": password,
		},
	}

	volume := VolumeData{
		Name:         volumeId,
		MountPoint:   "/var/lib/mysql",
		StorageClass: storageclass,
		Size:         sizes[0],
	}

	image := blueprint.ImageInfo{
		InternalPort: 3306,
		Image:        "mysql/mysql-server",
		Environment: map[string]string{
			"MYSQL_ROOT_HOST": "10.42.*.*",
		},
	}

	labels := map[string]string{"component": dsId}

	kubernetesClient, err := GetKubernetesClient(p.parent, deploymentID, infra.ID)
	if err != nil {
		logger.WithError(err).Error("Error getting kubernetes client")
		return err
	}

	secret := GetSecretDescription(secretData)

	logger.Info("Creating Secret")
	_, err = CreateOrUpdateSecret(logger, kubernetesClient, apiv1.NamespaceDefault, &secret)

	if err != nil {
		return err
	}
	logger.Info("Secret successfully created")

	podDescription := GetDatasourceDescription(dsId, 1, 30, labels, image, []SecretData{secretData}, []VolumeData{volume})

	logger.Info("Creating datasource pod")
	_, err = CreateOrUpdateStatefulSet(logger, kubernetesClient, apiv1.NamespaceDefault, &podDescription)

	if err != nil {
		return err
	}
	logger.Info("Datasource successfully created")

	dsService := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: dsId,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: labels,
			Ports: []corev1.ServicePort{
				corev1.ServicePort{
					Name: dsId,
					Port: int32(servicePort),
					TargetPort: intstr.IntOrString{
						IntVal: int32(3306),
					},
				},
			},
		},
	}

	logger.Info("Creating datasource service")
	_, err = CreateOrUpdateService(logger, kubernetesClient, apiv1.NamespaceDefault, &dsService)

	if err != nil {
		return err
	}
	logger.Info("Datasource service successfully created")

	mysqlDatasources[dsId] = servicePort
	infraInformation.Datasources["mysql"] = mysqlDatasources
	depInfo.InfraVDCs[infra.ID] = infraInformation

	return p.collection.FindOneAndReplace(context.Background(), bson.M{"deployment_id": deploymentID}, depInfo, nil).Decode(&depInfo)
}
