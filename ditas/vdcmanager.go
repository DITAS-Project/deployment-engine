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
			ID:        blueprintName,
			InfraVDCs: make(map[string]InfraServicesInformation),
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

	var infra model.InfrastructureDeploymentInfo
	for _, i := range deploymentInfo.Infrastructures {
		infra = i
		break
	}

	return m.DeployVDC(vdcInfo, bp, deploymentInfo.ID, infra)
}

func (m *VDCManager) provisionKubernetes(deployment model.DeploymentInfo, vdcInfo *VDCInformation) (model.DeploymentInfo, error) {
	result := deployment
	for _, infra := range deployment.Infrastructures {
		_, err := m.ProvisionerController.Provision(deployment.ID, infra.ID, "kubeadm", nil, "")
		//err := m.provisionKubernetesWithKubespray(deployment.ID, infra)
		if err != nil {
			log.WithError(err).Errorf("Error deploying kubernetes on infrastructure %s. Trying to clean up deployment.", infra.ID)
			return result, err
		}

		/*args := map[string][]string{
			ansible.AnsibleWaitForSSHReadyProperty: []string{"false"},
		}

		_, err = m.ProvisionerController.Provision(deployment.ID, infra.ID, "rook", args, "kubernetes")
		if err != nil {
			log.WithError(err).Error("Error deploying ceph cluster to master")
			return result, err
		}*/

		vdcInfo.InfraVDCs[infra.ID] = initializeVDCInformation()
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

func (m *VDCManager) DeployVDC(vdcInfo VDCInformation, blueprint blueprint.BlueprintType, deploymentID string, infra model.InfrastructureDeploymentInfo) error {
	blueprintName := *blueprint.InternalStructure.Overview.Name
	var err error

	if vdcInfo.ID != blueprintName {
		return fmt.Errorf("This cluster can only deploy blueprints \"%s\" but it got \"%s\"", vdcInfo.ID, blueprintName)
	}

	infraVdcs, ok := vdcInfo.InfraVDCs[infra.ID]
	if !ok {
		err := fmt.Errorf("Can't find infrastructure %s information for blueprint %s in deployment %s", infra.ID, blueprintName, vdcInfo.DeploymentID)
		log.WithError(err).Error("Error finding infrastructure information")
		return err
	}

	if !infraVdcs.Initialized {
		_, err = m.ProvisionerController.Provision(deploymentID, infra.ID, "vdm", nil, "kubernetes")
		if err != nil {
			log.WithError(err).Errorf("Error initializing infrastructure %s in deployment %s to host VDCs", infra.ID, vdcInfo.DeploymentID)
			return err
		}
		infraVdcs.Initialized = true
		vdcInfo.InfraVDCs[infra.ID] = infraVdcs

		err = m.Collection.FindOneAndReplace(context.Background(), bson.M{"_id": vdcInfo.ID}, vdcInfo).Decode(&vdcInfo)
		if err != nil {
			log.WithError(err).Errorf("Error updating infrastructure initialization")
			return err
		}
	}

	vdcID := fmt.Sprintf("vdc-%d", infraVdcs.VdcNumber)
	err = m.doDeployVDC(vdcInfo.DeploymentID, infra, blueprint, vdcID, infraVdcs.LastPort)

	if err != nil {
		log.WithError(err).Errorf("Error deploying VDC %s in infrastructure %s of deployment %s", vdcID, infra.ID, vdcInfo.DeploymentID)
		return err
	}

	infraVdcs.VdcNumber++
	infraVdcs.VdcPorts[vdcID] = infraVdcs.LastPort
	infraVdcs.LastPort++
	vdcInfo.InfraVDCs[infra.ID] = infraVdcs

	err = m.Collection.FindOneAndReplace(context.Background(), bson.M{"_id": vdcInfo.ID}, vdcInfo).Decode(&vdcInfo)
	if err != nil {
		log.WithError(err).Errorf("Error saving updated VDC information of deployment %s", vdcInfo.DeploymentID)
		return err
	}

	return nil
}

func (m *VDCManager) doDeployVDC(deploymentID string, infra model.InfrastructureDeploymentInfo, bp blueprint.BlueprintType, vdcID string, port int) error {

	args := make(model.Parameters)
	args["blueprint"] = bp
	args["vdcId"] = vdcID

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

func initializeVDCInformation() InfraServicesInformation {
	return InfraServicesInformation{
		Initialized:        false,
		LastPort:           30000,
		VdcNumber:          0,
		VdcPorts:           make(map[string]int),
		LastDatasourcePort: 40000,
		Datasources:        make(map[string]map[string]int),
	}
}
