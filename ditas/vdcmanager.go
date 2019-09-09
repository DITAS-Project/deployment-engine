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

	"net/url"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	DitasScriptsFolderProperty              = "ditas.folders.scripts"
	DitasConfigFolderProperty               = "ditas.folders.config"
	DitasRegistryURLProperty                = "ditas.registry.url"
	DitasRegistryUsernameProperty           = "ditas.registry.username"
	DitasRegistryPasswordProperty           = "ditas.registry.password"
	DitasPersistenceGlusterFSDeployProperty = "ditas.persistence.glusterfs.deploy"
	DitasPersistenceRookDeployProperty      = "ditas.persistence.rook.deploy"
	DitasTombstonePortProperty              = "ditas.tombstone.port"

	DitasScriptsFolderDefaultValue     = "ditas/scripts"
	DitasConfigFolderDefaultValue      = "ditas/VDC-Shared-Config"
	DitasPersistenceDeployDefaultValue = false
	DitasTombstonePortDefaultValue     = 30010

	ExtraPropertiesOwnerValue      = "owner"
	ApplicationDeveloperOwnerValue = "ApplicationDeveloper"
	DataAdministratorOwnerValue    = "DataAdministrator"
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
	viper.SetDefault(DitasPersistenceGlusterFSDeployProperty, DitasPersistenceDeployDefaultValue)
	viper.SetDefault(DitasPersistenceRookDeployProperty, DitasPersistenceDeployDefaultValue)
	viper.SetDefault(DitasTombstonePortProperty, DitasTombstonePortDefaultValue)

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

func (m *VDCManager) transformDeploymentInfo(src model.DeploymentInfo) (blueprint.DeploymentInfo, error) {
	var target blueprint.DeploymentInfo
	err := utils.TransformObject(src, &target)
	return target, err
}

// mergeDeployments creates a single deployment from the deployments associated to the data administrator and the application developer like this:
// If there's a deployment associated to the application developer, it will be used as base and the infrastructures associated to the data administrator (if they exist) will be appended
// If just the data administrator deployment is present, it will be used as the complete deployment
func (m *VDCManager) mergeDeployments(dataAdminDep model.DeploymentInfo, appDeveloperDep model.DeploymentInfo) model.DeploymentInfo {
	if appDeveloperDep.ID != "" {
		if dataAdminDep.ID != "" {
			for k, v := range dataAdminDep.Infrastructures {
				appDeveloperDep.Infrastructures[k] = v
			}
		}
		return appDeveloperDep
	} else {
		return dataAdminDep
	}
}

func (m *VDCManager) filterInfrastructures(ownerFilter string, infras []model.InfrastructureType) []model.InfrastructureType {
	result := make([]model.InfrastructureType, 0)
	for _, infra := range infras {
		owner, found := infra.ExtraProperties[ExtraPropertiesOwnerValue]
		if (!found && ownerFilter == DataAdministratorOwnerValue) || (found && owner == ownerFilter) {
			result = append(result, infra)
		}
	}
	return result
}

func (m *VDCManager) createDeployment(deployment model.Deployment) (model.DeploymentInfo, error) {
	deploymentInfo, err := m.DeploymentController.CreateDeployment(deployment)
	if err != nil {
		if deploymentInfo.ID != "" {
			errDelete := m.DeploymentController.DeleteDeployment(deploymentInfo.ID)
			if errDelete != nil {
				return deploymentInfo, fmt.Errorf("Error in deployment: %w and error cleaning deployment: %w", err, errDelete)
			}
			return deploymentInfo, fmt.Errorf("Error creating deployment: %w\nPartial deployment deleted", err)
		}
		return deploymentInfo, err
	}

	deploymentInfo, err = m.provisionKubernetes(deploymentInfo)
	if err != nil {
		log.WithError(err).Error("Error deploying kubernetes. Trying to clean deployment")

		/*for _, infra := range deploymentInfo.Infrastructures {
			_, err := m.DeploymentController.DeleteInfrastructure(deploymentInfo.ID, infra.ID)
			if err != nil {
				log.WithError(err).Errorf("Error deleting insfrastructure %s", infra.ID)
			}
		}*/
		return deploymentInfo, err
	}

	return deploymentInfo, nil
}

func (m *VDCManager) DeployVMD(deploymentID string, infra model.InfrastructureDeploymentInfo) (string, error) {
	_, err := m.ProvisionerController.Provision(deploymentID, infra.ID, "vdm", nil, "kubernetes")
	if err != nil {
		return "", utils.WrapLogAndReturnError(log.WithField("infrastructure", infra.ID), fmt.Sprintf("Error deploying VDM in infrastructure %s", infra.ID), err)
	}
	return infra.GetMasterIP()
}

func (m *VDCManager) DeployBlueprint(bp blueprint.Blueprint) (VDCInformation, error) {
	var vdcInfo VDCInformation
	if bp.ID == "" {
		return vdcInfo, errors.New("Invalid blueprint. Id is mandatory")
	}

	var originalDeployment model.Deployment
	err := utils.TransformObject(bp.CookbookAppendix.Resources, &originalDeployment)
	if err != nil {
		return vdcInfo, fmt.Errorf("Error getting resources information from blueprint: %w", err)
	}

	var dataOwnerDeployment model.DeploymentInfo
	err = m.Collection.FindOne(context.Background(), bson.M{"_id": bp.ID}).Decode(&vdcInfo)
	if err != nil {

		vdcInfo = VDCInformation{
			ID:      bp.ID,
			NumVDCs: 0,
			VDCs:    make(map[string]VDCConfiguration),
		}

		deployment := model.Deployment{
			Name:            originalDeployment.Name,
			Description:     originalDeployment.Description,
			Infrastructures: m.filterInfrastructures(DataAdministratorOwnerValue, originalDeployment.Infrastructures),
		}

		dataOwnerDeployment, err = m.createDeployment(deployment)
		if err != nil {
			return vdcInfo, fmt.Errorf("Error creating Data Administrator clusters: %w", err)
		}

		vdcInfo.DataOwnerDeploymentID = dataOwnerDeployment.ID

		_, err = m.Collection.InsertOne(context.Background(), vdcInfo)
		if err != nil {
			log.WithError(err).Error("Error saving blueprint VDC information")
			return vdcInfo, err
		}
	} else {
		if vdcInfo.DataOwnerDeploymentID != "" {
			dataOwnerDeployment, err = m.DeploymentController.Repository.GetDeployment(vdcInfo.DataOwnerDeploymentID)
			if err != nil {
				return vdcInfo, fmt.Errorf("Error getting data administrator deployment %s: %w", vdcInfo.DataOwnerDeploymentID, err)
			}
		} else {
			return vdcInfo, fmt.Errorf("Can't find data owner deployment in blueprint ID %s", bp.ID)
		}
	}

	if vdcInfo.VDMIP == "" {
		_, infra := m.findDefaultInfra(dataOwnerDeployment)

		vdmIP, err := m.DeployVMD(dataOwnerDeployment.ID, infra)
		if err != nil {
			return vdcInfo, fmt.Errorf("Error deploying VDM in infrastructure %s: %w", infra.ID, err)
		}
		vdcInfo.VDMIP = vdmIP
		vdcInfo.VDMInfraID = infra.ID
	}

	appDeveloperDeployment := model.Deployment{
		Name:            originalDeployment.Name,
		Description:     originalDeployment.Description,
		Infrastructures: m.filterInfrastructures(ApplicationDeveloperOwnerValue, originalDeployment.Infrastructures),
	}

	var appDeveloperDeploymentID string
	var appDeveloperDeploymentInfo model.DeploymentInfo
	if len(appDeveloperDeployment.Infrastructures) > 0 {
		appDeveloperDeploymentInfo, err := m.createDeployment(appDeveloperDeployment)
		if err != nil {
			return vdcInfo, fmt.Errorf("Error creating Application Developer cluster: %w", err)
		}
		appDeveloperDeploymentID = appDeveloperDeploymentInfo.ID
	}

	totalDeployment := m.mergeDeployments(dataOwnerDeployment, appDeveloperDeploymentInfo)
	deploymentID, infra := m.findDefaultInfra(totalDeployment)
	if deploymentID == "" || infra.ID == "" {
		return vdcInfo, errors.New("Can't find default infrastructure to deploy a new VDC")
	}

	vdcID := fmt.Sprintf("vdc-%d", vdcInfo.NumVDCs)

	bp.CookbookAppendix.Deployment, err = m.transformDeploymentInfo(totalDeployment)
	if err != nil {
		return vdcInfo, fmt.Errorf("Error transforming deployment information: %w", err)
	}

	vdmIP := ""
	if infra.ID != vdcInfo.VDMInfraID {
		vdmIP = vdcInfo.VDMIP
	}

	tombstonePort, _, err := m.DeployVDC(bp, deploymentID, infra, vdcID, vdmIP)
	if err != nil {
		return vdcInfo, err
	}

	strBp, err := json.Marshal(bp)
	if err != nil {
		return vdcInfo, fmt.Errorf("Error marshaling blueprint: %w", err)
	}

	masterIP, err := infra.GetMasterIP()
	if err != nil {
		return vdcInfo, fmt.Errorf("Error getting master node of infrastructure %s: %w", infra.ID, err)
	}

	config := VDCConfiguration{
		Blueprint: string(strBp),
		Infrastructures: map[string]InfrastructureInformation{
			infra.ID: InfrastructureInformation{
				IP:            masterIP,
				TombstonePort: tombstonePort,
			},
		},
		AppDeveloperDeploymentID: appDeveloperDeploymentID,
	}

	vdcInfo.VDCs[vdcID] = config
	vdcInfo.NumVDCs++

	_, err = m.Collection.ReplaceOne(context.Background(), bson.M{"_id": vdcInfo.ID}, vdcInfo, options.Replace())
	if err != nil {
		return vdcInfo, fmt.Errorf("Error saving VDC information: %w", err)
	}

	return vdcInfo, err
}

func (m *VDCManager) findDefaultInfra(deployments ...model.DeploymentInfo) (string, model.InfrastructureDeploymentInfo) {
	var deploymentID string
	var infra model.InfrastructureDeploymentInfo
	for _, deployment := range deployments {
		deploymentID = deployment.ID
		if deployment.Infrastructures != nil && len(deployment.Infrastructures) > 0 {
			for _, v := range deployment.Infrastructures {
				infra = v
				if v.ExtraProperties.GetBool("ditas_default") {
					return deploymentID, v
				}
			}
		}
	}

	return deploymentID, infra
}

/*func (m *VDCManager) provisionPersistence(solution, property string, deployment model.Deployment, infra model.InfrastructureDeploymentInfo) (model.DeploymentInfo, error) {
	if viper.GetBool(property) {
		args := map[string][]string{
			ansible.AnsibleWaitForSSHReadyProperty: []string{"false"},
		}

		return m.ProvisionerController.Provision(deployment.ID, infra.ID, solution, args, "kubernetes")
	}
	return deployment, nil
}*/

func (m *VDCManager) provisionKubernetes(deployment model.DeploymentInfo) (model.DeploymentInfo, error) {
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

		args = model.Parameters{
			ansible.AnsibleWaitForSSHReadyProperty: false,
		}

		result, err = m.ProvisionerController.Provision(deployment.ID, infra.ID, "helm", args, "")
		if err != nil {
			log.WithError(err).Errorf("Error deploying helm in infrastructure %s", infra.ID)
			return result, err
		}

		vars, err := utils.GetVarsFromConfigFolder()
		if err != nil {
			log.WithError(err).Error("Error reading variables from configuration folder")
			return result, err
		}

		esURL, ok := vars[ElasticSearchUrlVarName]
		if ok {

			parsedURL, err := url.Parse(esURL.(string))
			if err != nil {
				log.WithError(err).Errorf("Invalid elasticsearch URL: %s", parsedURL)
				return result, err
			}

			args["elasticsearch.host"] = parsedURL.Hostname()
			args["elasticsearch.port"] = parsedURL.Port()

			esUsername, ok := vars[ElasticSearchUsernameVarName]
			if ok {
				args["elasticsearch.auth.enabled"] = "true"
				args["elasticsearch.auth.user"] = esUsername

				esPassword, ok := vars[ElasticSearchPasswordVarName]
				if ok {
					args["elasticsearch.auth.password"] = esPassword
				}
			}

			result, err = m.ProvisionerController.Provision(deployment.ID, infra.ID, "fluentd", args, "")
			if err != nil {
				log.WithError(err).Errorf("Error installing fluentd at infrastructure %s", infra.ID)
				return result, err
			}
		}

		result, err = m.ProvisionerController.Provision(deployment.ID, infra.ID, "rook", args, "kubernetes")
		if err != nil {
			log.WithError(err).Errorf("Error deploying rook to kubernetes cluster %s of deployment %s", infra.ID, deployment.ID)
			return result, err
		}

	}
	return result, nil
}

func (m *VDCManager) DeployVDC(blueprint blueprint.Blueprint, deploymentID string, infra model.InfrastructureDeploymentInfo, vdcID, vdmIP string) (int, model.DeploymentInfo, error) {

	var deployment model.DeploymentInfo
	kubeConfigRaw, ok := infra.Products["kubernetes"]
	if !ok {
		return -1, deployment, fmt.Errorf("Infrastructure %s doesn't have kubernetes installed", infra.ID)
	}

	var kubeconfig kubernetes.KubernetesConfiguration
	err := utils.TransformObject(kubeConfigRaw, &kubeconfig)
	if err != nil {
		return -1, deployment, fmt.Errorf("Error reading kubernetes configuration from infrastructure %s: %w", infra.ID, err)
	}

	tombstonePort := kubeconfig.GetNewFreePort()

	deployment, err = m.doDeployVDC(deploymentID, infra, blueprint, vdcID, vdmIP, tombstonePort)
	return tombstonePort, deployment, err
}

func (m *VDCManager) doDeployVDC(deploymentID string, infra model.InfrastructureDeploymentInfo, bp blueprint.Blueprint, vdcID, vdmIP string, tombstonePort int) (model.DeploymentInfo, error) {

	args := make(model.Parameters)
	args["blueprint"] = bp
	args["vdcId"] = vdcID
	args["tombstonePort"] = tombstonePort
	args["vdmIP"] = vdmIP
	return m.ProvisionerController.Provision(deploymentID, infra.ID, "vdc", args, "kubernetes")
}

func (m *VDCManager) FindInfrastructureDeployment(vdcsInfo VDCInformation, vdcID, infraID string) (string, error) {
	if vdcsInfo.DataOwnerDeploymentID != "" {
		_, err := m.DeploymentController.Repository.FindInfrastructure(vdcsInfo.DataOwnerDeploymentID, infraID)
		if err == nil {
			return vdcsInfo.DataOwnerDeploymentID, nil
		}
	}

	vdcConfig, ok := vdcsInfo.VDCs[vdcID]
	if !ok {
		return "", fmt.Errorf("Can't find configuration of VDC %s", vdcID)
	}

	if vdcConfig.AppDeveloperDeploymentID != "" {
		_, err := m.DeploymentController.Repository.FindInfrastructure(vdcConfig.AppDeveloperDeploymentID, infraID)
		if err == nil {
			return vdcConfig.AppDeveloperDeploymentID, nil
		}
	}

	return "", fmt.Errorf("Can't find infrastructure %s associated to any deployment of blueprint %s", infraID, vdcsInfo.ID)
}

func (m *VDCManager) CopyVDC(blueprintID, vdcID, targetInfraID string) (VDCConfiguration, error) {
	var vdcInfo VDCInformation
	var vdcConfig VDCConfiguration
	var targetDeployment model.DeploymentInfo
	err := m.Collection.FindOne(context.Background(), bson.M{"_id": blueprintID}).Decode(&vdcInfo)
	if err != nil {
		return vdcConfig, fmt.Errorf("Error finding deployment for blueprint %s: %w", blueprintID, err)
	}

	vdcConfig, ok := vdcInfo.VDCs[vdcID]
	if !ok {
		return vdcConfig, fmt.Errorf("Can't find VDC with identifier %s", vdcID)
	}

	for infraID := range vdcConfig.Infrastructures {
		if infraID == targetInfraID {
			return vdcConfig, fmt.Errorf("VDC %s is already running in infrastructure %s", vdcID, targetInfraID)
		}
	}

	daDeployment, err := m.DeploymentController.Repository.GetDeployment(vdcInfo.DataOwnerDeploymentID)
	if err != nil {
		return vdcConfig, fmt.Errorf("Error getting data owner deployment: %w", err)
	}

	_, ok = daDeployment.Infrastructures[targetInfraID]
	if ok {
		targetDeployment = daDeployment
	}

	if targetDeployment.ID == "" {
		deploymentID, err := m.FindInfrastructureDeployment(vdcInfo, vdcID, targetInfraID)
		if err != nil {
			return vdcConfig, err
		}
		targetDeployment, err = m.DeploymentController.Repository.GetDeployment(deploymentID)
		if err != nil {
			return vdcConfig, fmt.Errorf("Error gettind deployment %s associated to blueprint %s: %w", deploymentID, blueprintID, err)
		}
	}

	var bp blueprint.Blueprint
	err = json.Unmarshal([]byte(vdcConfig.Blueprint), &bp)
	if err != nil {
		return vdcConfig, fmt.Errorf("Error unmarshaling blueprint for VDC %s: %w", vdcID, err)
	}

	targetInfra, ok := targetDeployment.Infrastructures[targetInfraID]
	if !ok {
		return vdcConfig, fmt.Errorf("Can't find target infrastructure %s in target deployment %s. Weird", targetInfraID, targetDeployment.ID)
	}

	vdmIP := ""
	if targetInfraID != vdcInfo.VDMInfraID {
		vdmIP = vdcInfo.VDMIP
	}

	tombstonePort, targetDeployment, err := m.DeployVDC(bp, targetDeployment.ID, targetInfra, vdcID, vdmIP)
	if err != nil {
		return vdcConfig, fmt.Errorf("Error creating copy of VDC %s: %w", vdcID, err)
	}

	masterIP, err := targetInfra.GetMasterIP()
	if err != nil {
		return vdcConfig, fmt.Errorf("Error getting master IP of infrastructure %s: %w", targetInfraID, err)
	}

	vdcConfig.Infrastructures[targetInfraID] = InfrastructureInformation{
		IP:            masterIP,
		TombstonePort: tombstonePort,
	}

	vdcInfo.VDCs[vdcID] = vdcConfig

	updRes := m.Collection.FindOneAndReplace(context.Background(), bson.M{"_id": blueprintID}, vdcInfo, options.FindOneAndReplace())

	if updRes.Err() != nil {
		return vdcConfig, fmt.Errorf("Error updating VDC information for blueprint %s: %w", blueprintID, updRes.Err())
	}

	return vdcConfig, nil
}

func (m *VDCManager) doDeleteVDC(infra *model.InfrastructureDeploymentInfo, vdcID string) error {
	logger := log.WithFields(log.Fields{
		"infra": infra.ID,
		"vdc":   vdcID,
	})

	kubeConfig, err := kubernetes.GetKubernetesConfiguration(*infra)
	if err != nil {
		return fmt.Errorf("Error reading kuberentes configuration from infrastructure %s: %s", infra.ID, err.Error())
	}

	kubeClient, err := kubernetes.NewClient(kubeConfig.ConfigurationFile)
	if err != nil {
		return err
	}

	logger.Info("Deleting VDC")
	err = kubeClient.Client.AppsV1().Deployments(apiv1.NamespaceDefault).Delete(vdcID, &metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("Error deleting VDC %s: %s", vdcID, err.Error())
	}

	return err
}

func (m *VDCManager) findVDMIP(deployment model.DeploymentInfo) string {
	for _, infra := range deployment.Infrastructures {
		kubeConfig, err := kubernetes.GetKubernetesConfiguration(infra)
		if err == nil {
			_, ok := kubeConfig.DeploymentsConfiguration["VDM"]
			if ok {
				master, err := infra.GetFirstNodeOfRole("master")
				if err == nil {
					return master.IP
				}
			}
		}
	}
	return ""
}

func (m *VDCManager) DeployDatasource(blueprintId, vdcID, infraId, datasourceType string, args model.Parameters) error {
	var blueprintInfo VDCInformation
	err := m.Collection.FindOne(context.Background(), bson.M{"_id": blueprintId}).Decode(&blueprintInfo)
	if err != nil {
		return fmt.Errorf("Can't find information for blueprint %s: %s", blueprintId, err.Error())
	}

	deploymentID, err := m.FindInfrastructureDeployment(blueprintInfo, vdcID, infraId)
	if err != nil {
		return err
	}

	infra, err := m.DeploymentController.Repository.FindInfrastructure(deploymentID, infraId)
	if err != nil {
		return fmt.Errorf("Can't finde infrastructure %s in deployment %s associated to blueprint %s: %v", infraId, deploymentID, blueprintId, err)
	}

	_, err = m.ProvisionerController.Provision(deploymentID, infra.ID, datasourceType, args, "kubernetes")
	return err
}

func (m *VDCManager) GetVDCInformation(blueprintID, vdcID string) (VDCConfiguration, error) {
	var vdcInfo VDCInformation
	var result VDCConfiguration
	err := m.Collection.FindOne(context.Background(), bson.M{"_id": blueprintID}, options.FindOne()).Decode(&vdcInfo)
	if err != nil {
		return result, fmt.Errorf("Error getting blueprint %s information: %w", blueprintID, err)
	}

	result, ok := vdcInfo.VDCs[vdcID]
	if !ok {
		return result, fmt.Errorf("Can't find VDC %s in blueprint information %s", blueprintID, vdcID)
	}

	return result, nil
}
