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
	"encoding/base64"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"

	"go.mongodb.org/mongo-driver/mongo"

	"github.com/sethvargo/go-password/password"
	"github.com/sirupsen/logrus"
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
	servicePort := infraInformation.LastDatasourcePort
	password, err := password.Generate(10, 3, 2, false, false)

	if err != nil {
		return err
	}

	encodedPassword := base64.StdEncoding.EncodeToString([]byte(password))

	storageclass := "rook-ceph-block-single"

	err = ansible.ExecutePlaybook(logger, p.scriptsFolder+"/deploy_datasource.yml", inventoryPath, map[string]string{
		"mysql_id":                dsId,
		"mysql_service_port":      fmt.Sprintf("%d", servicePort),
		"mysql_enconded_password": encodedPassword,
		"storage_class":           storageclass,
		"datasource":              "mysql",
	})

	if err != nil {
		return err
	}

	mysqlDatasources[dsId] = servicePort
	infraInformation.Datasources["mysql"] = mysqlDatasources
	depInfo.InfraVDCs[infra.ID] = infraInformation

	return p.collection.FindOneAndReplace(context.Background(), bson.M{"deployment_id": deploymentID}, depInfo, nil).Decode(&depInfo)
}
