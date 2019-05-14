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
	"deployment-engine/infrastructure"
	"deployment-engine/model"
	"deployment-engine/persistence/mongorepo"
	"deployment-engine/provision"
	"deployment-engine/provision/ansible"
	"deployment-engine/provision/kubernetes"
	"deployment-engine/utils"
	"encoding/json"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/mongo/options"

	blueprint "github.com/DITAS-Project/blueprint-go"
	"go.mongodb.org/mongo-driver/mongo"
	"gopkg.in/mgo.v2/bson"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

const (
	DitasScriptsFolderProperty    = "ditas.folders.scripts"
	DitasConfigFolderProperty     = "ditas.folders.config"
	DitasRegistryURLProperty      = "ditas.registry.url"
	DitasRegistryUsernameProperty = "ditas.registry.username"
	DitasRegistryPasswordProperty = "ditas.registry.password"

	DitasScriptsFolderDefaultValue = "ditas/scripts"
	DitasConfigFolderDefaultValue  = "ditas/VDC-Shared-Config"
)

type VDCManager struct {
	Collection            *mongo.Collection
	ScriptsFolder         string
	ConfigFolder          string
	ConfigVariablesPath   string
	DeploymentController  *infrastructure.Deployer
	ProvisionerController *provision.ProvisionerController
}

func NewVDCManager(deployer *infrastructure.Deployer, provisionerController *provision.ProvisionerController) (*VDCManager, error) {
	viper.SetDefault(mongorepo.MongoDBURLName, mongorepo.MongoDBURLDefault)
	viper.SetDefault(DitasScriptsFolderProperty, DitasScriptsFolderDefaultValue)
	viper.SetDefault(DitasConfigFolderProperty, DitasConfigFolderDefaultValue)

	configFolder, err := utils.ConfigurationFolder()
	if err != nil {
		log.WithError(err).Errorf("Error getting configuration folder")
		return nil, err
	}

	mongoConnectionURL := viper.GetString(mongorepo.MongoDBURLName)
	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI(mongoConnectionURL))
	if err != nil {
		log.WithError(err).Errorf("Error connecting to MongoDB server %s", mongoConnectionURL)
		return nil, err
	}

	db := client.Database("deployment_engine")
	if db == nil {
		log.WithError(err).Error("Error getting deployment engine database")
		return nil, err
	}

	scriptsFolder := viper.GetString(DitasScriptsFolderProperty)
	configVarsPath := configFolder + "/vars.yml"
	ditasPodsConfigFolder := viper.GetString(DitasConfigFolderProperty)
	vdcCollection := db.Collection("vdcs")

	kubeProvisioner := kubernetes.NewKubernetesController()
	kubeProvisioner.AddProvisioner("vdm", NewVDMProvisioner(scriptsFolder, configVarsPath, ditasPodsConfigFolder))
	kubeProvisioner.AddProvisioner("vdc", NewVDCProvisioner(ditasPodsConfigFolder))

	provisionerController.Provisioners["kubernetes"] = kubeProvisioner

	return &VDCManager{
		Collection:            vdcCollection,
		ScriptsFolder:         scriptsFolder,
		ConfigFolder:          ditasPodsConfigFolder,
		ConfigVariablesPath:   configVarsPath,
		DeploymentController:  deployer,
		ProvisionerController: provisionerController,
	}, nil
}

func (m *VDCManager) DeployBlueprint(request CreateDeploymentRequest) error {
	bp := request.Blueprint
	if bp.InternalStructure.Overview.Name == nil {
		return errors.New("Invalid blueprint. Name is mandatory")
	}
	blueprintName := *bp.InternalStructure.Overview.Name
	var vdcInfo VDCInformation
	var deploymentInfo model.DeploymentInfo
	err := m.Collection.FindOne(context.Background(), bson.M{"_id": blueprintName}).Decode(&vdcInfo)
	if err != nil {

		vdcInfo = VDCInformation{
			ID: blueprintName,
		}

		deployment, err := m.getDeployment(&bp, request.Resources)
		if err != nil {
			return err
		}

		deploymentInfo, err = m.DeploymentController.CreateDeployment(*deployment)
		if err != nil {
			return err
		}

		vdcInfo.DeploymentID = deploymentInfo.ID

		deploymentInfo, err = m.provisionKubernetes(deploymentInfo, &vdcInfo)
		if err != nil {
			log.WithError(err).Error("Error deploying kubernetes. Trying to clean deployment")

			/*for _, infra := range deploymentInfo.Infrastructures {
				_, err := m.DeploymentController.DeleteInfrastructure(deploymentInfo.ID, infra.ID)
				if err != nil {
					log.WithError(err).Errorf("Error deleting insfrastructure %s", infra.ID)
				}
			}*/
			return err
		}

		_, err = m.Collection.InsertOne(context.Background(), vdcInfo)
		if err != nil {
			log.WithError(err).Error("Error saving blueprint VDC information")
			return err
		}
	}

	if deploymentInfo.ID == "" {
		deploymentInfo, err = m.DeploymentController.Repository.GetDeployment(vdcInfo.DeploymentID)
		if err != nil {
			log.WithError(err).Errorf("Error finding deployment %s for blueprint %s", vdcInfo.DeploymentID, vdcInfo.ID)
			return err
		}
	}

	infra, ok := m.findDefaultInfra(deploymentInfo)
	if !ok {
		return fmt.Errorf("Can't find default infrastructure in deployment %s", deploymentInfo.ID)
	}

	return m.DeployVDC(bp, deploymentInfo.ID, infra)
}

func (m *VDCManager) findDefaultInfra(deployment model.DeploymentInfo) (model.InfrastructureDeploymentInfo, bool) {
	if deployment.Infrastructures != nil && len(deployment.Infrastructures) > 0 {
		var random model.InfrastructureDeploymentInfo
		for _, v := range deployment.Infrastructures {
			random = v
			if v.ExtraProperties.GetBool("ditas_default") {
				return v, true
			}
		}
		// If we don't find the default one, we return the last one in the loop.
		return random, true
	}
	return model.InfrastructureDeploymentInfo{}, false
}

func (m *VDCManager) provisionKubernetes(deployment model.DeploymentInfo, vdcInfo *VDCInformation) (model.DeploymentInfo, error) {
	var result model.DeploymentInfo
	var err error
	for _, infra := range deployment.Infrastructures {
		args := make(model.Parameters)
		args[ansible.AnsibleWaitForSSHReadyProperty] = true
		result, err = m.ProvisionerController.Provision(deployment.ID, infra.ID, "kubeadm", args, "")
		//err := m.provisionKubernetesWithKubespray(deployment.ID, infra)
		if err != nil {
			log.WithError(err).Errorf("Error deploying kubernetes on infrastructure %s. Trying to clean up deployment.", infra.ID)
			return result, err
		}

		/*args := map[string][]string{
			ansible.AnsibleWaitForSSHReadyProperty: []string{"false"},
		}

		result, err = m.ProvisionerController.Provision(deployment.ID, infra.ID, "rook", args, "kubernetes")
		if err != nil {
			log.WithError(err).Error("Error deploying ceph cluster to master")
			return result, err
		}*/
	}
	return result, nil
}

func (m *VDCManager) getDeployment(bp *blueprint.BlueprintType, infrastructures []blueprint.InfrastructureType) (*model.Deployment, error) {

	appendix := blueprint.CookbookAppendix{
		Name:            *bp.InternalStructure.Overview.Name,
		Infrastructures: infrastructures,
	}

	bp.CookbookAppendix = appendix

	appendixStr, err := json.Marshal(appendix)
	if err != nil {
		log.WithError(err).Error("Can't marshall Cookbook Appendix")
		return nil, err
	}

	var deployment model.Deployment
	err = json.Unmarshal(appendixStr, &deployment)
	if err != nil {
		log.WithError(err).Error("Can't unmarshal Cookbook Appendix into Deployment")
	}
	return &deployment, err
}

func (m *VDCManager) DeployVDC(blueprint blueprint.BlueprintType, deploymentID string, infra model.InfrastructureDeploymentInfo) error {

	kubeConfigRaw, ok := infra.Products["kubernetes"]
	if !ok {
		return fmt.Errorf("Infrastructure %s doesn't have kubernetes installed", infra.ID)
	}

	var kubeconfig kubernetes.KubernetesConfiguration
	err := utils.TransformObject(kubeConfigRaw, &kubeconfig)
	if err != nil {
		return fmt.Errorf("Error reading kubernetes configuration from infrastructure %s: %s", infra.ID, err.Error())
	}

	_, initialized := kubeconfig.DeploymentsConfiguration["VDM"]

	if !initialized {
		_, err = m.ProvisionerController.Provision(deploymentID, infra.ID, "vdm", nil, "kubernetes")
		if err != nil {
			log.WithError(err).Errorf("Error initializing infrastructure %s in deployment %s to host VDCs", infra.ID, deploymentID)
			return err
		}
	}

	return m.doDeployVDC(deploymentID, infra, blueprint)
}

func (m *VDCManager) doDeployVDC(deploymentID string, infra model.InfrastructureDeploymentInfo, bp blueprint.BlueprintType) error {

	args := make(model.Parameters)
	args["blueprint"] = bp

	_, err := m.ProvisionerController.Provision(deploymentID, infra.ID, "vdc", args, "kubernetes")
	return err
}

func (m *VDCManager) DeployDatasource(blueprintId, infraId, datasourceType string, args model.Parameters) error {
	var blueprintInfo VDCInformation
	err := m.Collection.FindOne(context.Background(), bson.M{"_id": blueprintId}).Decode(&blueprintInfo)
	if err != nil {
		return fmt.Errorf("Can't find information for blueprint %s: %s", blueprintId, err.Error())
	}

	infra, err := m.DeploymentController.Repository.FindInfrastructure(blueprintInfo.DeploymentID, infraId)
	if err != nil {
		return fmt.Errorf("Can't finde infrastructure %s in deployment %s associated to blueprint %s: %s", infraId, blueprintInfo.DeploymentID, blueprintId, err.Error())
	}

	_, err = m.ProvisionerController.Provision(blueprintInfo.DeploymentID, infra.ID, datasourceType, args, "kubernetes")
	return err
}
