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
	"deployment-engine/utils"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"

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
	DitasKubesprayFolderProperty  = "ditas.folders.kubespray"
	DitasRegistryURLProperty      = "ditas.registry.url"
	DitasRegistryUsernameProperty = "ditas.registry.username"
	DitasRegistryPasswordProperty = "ditas.registry.password"

	DitasScriptsFolderDefaultValue   = "ditas/scripts"
	DitasConfigFolderDefaultValue    = "ditas/VDC-Shared-Config"
	DitasKubesprayFolderDefaultValue = "ditas/kubespray"
)

type VDCManager struct {
	Collection            *mongo.Collection
	ScriptsFolder         string
	ConfigFolder          string
	ConfigVariablesPath   string
	KubesprayFolder       string
	DeploymentController  *infrastructure.Deployer
	ProvisionerController *provision.ProvisionerController
	Provisioner           *ansible.Provisioner
}

func NewVDCManager(provisioner *ansible.Provisioner, deployer *infrastructure.Deployer, provisionerController *provision.ProvisionerController) (*VDCManager, error) {
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
	kubesprayFolder := viper.GetString(DitasKubesprayFolderProperty)
	configVarsPath := configFolder + "/vars.yml"
	ditasPodsConfigFolder := viper.GetString(DitasConfigFolderProperty)
	vdcCollection := db.Collection("vdcs")
	registry := Registry{
		URL:      viper.GetString(DitasRegistryURLProperty),
		Username: viper.GetString(DitasRegistryUsernameProperty),
		Password: viper.GetString(DitasRegistryPasswordProperty),
	}

	provisioner.Provisioners["glusterfs"] = NewGlusterfsProvisioner(provisioner, scriptsFolder)
	provisioner.Provisioners["k3s"] = NewK3sProvisioner(provisioner, scriptsFolder, registry)
	provisioner.Provisioners["kubeadm"] = NewKubeadmProvisioner(provisioner, scriptsFolder)
	provisioner.Provisioners["kubespray"] = NewKubesprayProvisioner(provisioner, kubesprayFolder)
	provisioner.Provisioners["rook"] = NewRookProvisioner(provisioner, scriptsFolder)
	provisioner.Provisioners["vdm"] = NewVDMProvisioner(provisioner, scriptsFolder, configVarsPath, ditasPodsConfigFolder, registry)
	provisioner.Provisioners["mysql"] = NewMySQLProvisioner(provisioner, scriptsFolder, vdcCollection)

	return &VDCManager{
		Collection:            vdcCollection,
		ScriptsFolder:         scriptsFolder,
		ConfigFolder:          ditasPodsConfigFolder,
		KubesprayFolder:       kubesprayFolder,
		ConfigVariablesPath:   configVarsPath,
		Provisioner:           provisioner,
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

	return m.DeployVDC(vdcInfo, bp, deploymentInfo.ID, deploymentInfo.Infrastructures[0])
}

func (m *VDCManager) provisionKubernetes(deployment model.DeploymentInfo, vdcInfo *VDCInformation) (model.DeploymentInfo, error) {
	result := deployment
	for _, infra := range deployment.Infrastructures {
		err := m.Provisioner.Provision(deployment.ID, infra, "k3s", nil)
		//err := m.provisionKubernetesWithKubespray(deployment.ID, infra)
		if err != nil {
			log.WithError(err).Errorf("Error deploying kubernetes on infrastructure %s. Trying to clean up deployment.", infra.ID)
			return result, err
		}

		/*err = m.Provisioner.WaitAndProvision(deployment.ID, infra, "rook", false, nil)
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
		err = m.Provisioner.Provision(deploymentID, infra, "vdm", nil)
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
	logger := log.WithField("blueprint", *bp.InternalStructure.Overview.Name)
	blueprintPath, err := m.writeBlueprint(logger, bp, vdcID, deploymentID, infra.ID)
	if err != nil {
		logger.WithError(err).Error("Error writing blueprint")
		return err
	}

	return m.executePlaybook(deploymentID, infra, "deploy_vdc.yml", map[string]string{
		"vdcId":          vdcID,
		"vars_file":      m.ConfigVariablesPath,
		"blueprint_path": blueprintPath,
		"config_folder":  m.ConfigFolder,
		"master_ip":      infra.Master.IP,
		"internalPort":   strconv.Itoa(port + 20000),
		"vdcPort":        strconv.Itoa(port),
	})
}

func (m *VDCManager) executePlaybook(deploymentID string, infra model.InfrastructureDeploymentInfo, playbook string, extravars map[string]string) error {
	inventory := m.Provisioner.GetInventoryPath(deploymentID, infra.ID)
	return ansible.ExecutePlaybook(log.WithField("deployment", deploymentID).WithField("infrastructure", infra.ID), m.ScriptsFolder+"/"+playbook, inventory, extravars)
}

func (m *VDCManager) writeBlueprint(logger *log.Entry, bp blueprint.BlueprintType, vdcID, deploymentID, infraID string) (string, error) {
	path := m.Provisioner.GetInventoryFolder(deploymentID, infraID) + "/" + vdcID

	err := os.MkdirAll(path, os.ModePerm)
	if err != nil {
		logger.WithError(err).Errorf("Error creating infrastructure blueprints folder %s", path)
		return "", err
	}

	name := path + "/blueprint.json"
	logger.Infof("Copying blueprint to %s", name)

	jsonData, err := json.Marshal(bp)
	jsonFile, err := os.Create(name)
	if err != nil {
		logger.WithError(err).Errorf("Error creating blueprint file %s", name)
		return name, err
	}
	defer jsonFile.Close()
	_, err = jsonFile.Write(jsonData)
	if err != nil {
		logger.WithError(err).Errorf("Error writing blueprint file %s", name)
		return name, err
	}

	logger.Info("Blueprint copied")

	return name, nil
}

func (m *VDCManager) DeployDatasource(blueprintId, infraId, datasourceType string, args map[string][]string) error {
	var blueprintInfo VDCInformation
	err := m.Collection.FindOne(context.Background(), bson.M{"_id": blueprintId}).Decode(&blueprintInfo)
	if err != nil {
		return fmt.Errorf("Can't find information for blueprint %s: %s", blueprintId, err.Error())
	}

	infra, err := m.DeploymentController.Repository.FindInfrastructure(blueprintInfo.DeploymentID, infraId)
	if err != nil {
		return fmt.Errorf("Can't finde infrastructure %s in deployment %s associated to blueprint %s: %s", infraId, blueprintInfo.DeploymentID, blueprintId, err.Error())
	}

	wait := true
	waitList, ok := args["wait"]
	if ok && len(waitList) > 0 {
		wait, _ = strconv.ParseBool(waitList[0])
	}

	return m.Provisioner.WaitAndProvision(blueprintInfo.DeploymentID, infra, datasourceType, wait, args)
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
