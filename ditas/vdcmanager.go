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

	"net/url"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	DitasScriptsFolderProperty              = "ditas.folders.scripts"
	DitasConfigFolderProperty               = "ditas.folders.config"
	DitasPersistenceGlusterFSDeployProperty = "ditas.persistence.glusterfs.deploy"
	DitasPersistenceRookDeployProperty      = "ditas.persistence.rook.deploy"
	DitasVariablesProperty                  = "ditas.variables"

	DitasScriptsFolderDefaultValue     = "ditas/scripts"
	DitasConfigFolderDefaultValue      = "ditas/VDC-Shared-Config"
	DitasPersistenceDeployDefaultValue = false

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

type KubernetesProvisionResult struct {
	Infra model.InfrastructureDeploymentInfo
	Error error
}

func NewVDCManager(deployer *infrastructure.Deployer, provisionerController *provision.ProvisionerController) (*VDCManager, error) {
	viper.SetDefault(mongorepo.MongoDBURLName, mongorepo.MongoDBURLDefault)
	viper.SetDefault(DitasScriptsFolderProperty, DitasScriptsFolderDefaultValue)
	viper.SetDefault(DitasConfigFolderProperty, DitasConfigFolderDefaultValue)
	viper.SetDefault(DitasPersistenceGlusterFSDeployProperty, DitasPersistenceDeployDefaultValue)
	viper.SetDefault(DitasPersistenceRookDeployProperty, DitasPersistenceDeployDefaultValue)

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

func (m *VDCManager) toIDs(src model.DeploymentInfo) []string {
	result := make([]string, len(src))
	for i, infra := range src {
		result[i] = infra.ID
	}
	return result
}

func (m *VDCManager) toInfras(ids []string) (model.DeploymentInfo, error) {
	result := make(model.DeploymentInfo, len(ids))
	for i, ID := range ids {
		infra, err := m.DeploymentController.Repository.FindInfrastructure(ID)
		if err != nil {
			return result, fmt.Errorf("Error retrieving infrastructure %s: %w", ID, err)
		}
		result[i] = infra
	}
	return result, nil
}

func (m *VDCManager) transformDeploymentInfo(ID string, src model.DeploymentInfo) (blueprint.DeploymentInfo, error) {
	target := blueprint.DeploymentInfo{
		ID:              ID,
		Infrastructures: make(map[string]blueprint.InfrastructureDeploymentInfo),
	}
	var bpInfras []blueprint.InfrastructureDeploymentInfo
	err := utils.TransformObject(src, &bpInfras)
	if err != nil {
		return target, fmt.Errorf("Error transforming deployment to blueprint format: %w", err)
	}
	for _, infra := range bpInfras {
		target.Infrastructures[infra.ID] = infra
	}
	return target, err
}

// mergeDeployments creates a single deployment from the deployments associated to the data administrator and the application developer by appending one to another
func (m *VDCManager) mergeDeployments(dataAdminDep model.DeploymentInfo, appDeveloperDep model.DeploymentInfo) model.DeploymentInfo {
	result := make(model.DeploymentInfo, 0, len(dataAdminDep)+len(appDeveloperDep))
	result = append(result, dataAdminDep...)
	return append(result, appDeveloperDep...)
}

func (m *VDCManager) filterInfrastructures(ownerFilter string, infras []blueprint.InfrastructureType) []blueprint.InfrastructureType {
	result := make([]blueprint.InfrastructureType, 0)
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
		toDelete := make([]string, len(deploymentInfo))
		for i, infra := range deploymentInfo {
			toDelete[i] = infra.ID
		}
		errDelete := m.DeploymentController.DeleteDeployment(toDelete)
		if errDelete != nil {
			return deploymentInfo, fmt.Errorf("Error in deployment: %w and error cleaning deployment: %w", err, errDelete)
		}
		return deploymentInfo, fmt.Errorf("Error creating deployment: %w Partial deployment deleted", err)
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

func (m *VDCManager) DeployVMD(infra model.InfrastructureDeploymentInfo, blueprintID string) (string, error) {
	args := make(model.Parameters)
	args[BlueprintIDProperty] = blueprintID
	args[VariablesProperty] = m.getVarsFromConfig()
	_, _, err := m.ProvisionerController.Provision(infra.ID, "vdm", args, "kubernetes")
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

	var dataOwnerDeployment model.DeploymentInfo
	err := m.Collection.FindOne(context.Background(), bson.M{"_id": bp.ID}).Decode(&vdcInfo)
	if err != nil {

		vdcInfo = VDCInformation{
			ID:      bp.ID,
			NumVDCs: 0,
			VDCs:    make(map[string]VDCConfiguration),
		}

		dataOwnerDeploymentOriginal := m.filterInfrastructures(DataAdministratorOwnerValue, bp.CookbookAppendix.Resources.Infrastructures)

		var deployment model.Deployment
		err = utils.TransformObject(dataOwnerDeploymentOriginal, &deployment)
		if err != nil {
			return vdcInfo, fmt.Errorf("Error transforming resources from blueprint: %w", err)
		}

		dataOwnerDeployment, err = m.createDeployment(deployment)
		if err != nil {
			return vdcInfo, fmt.Errorf("Error creating Data Administrator clusters: %w", err)
		}

		vdcInfo.DataOwnerDeployment = make([]string, len(dataOwnerDeployment))
		for i, infra := range dataOwnerDeployment {
			vdcInfo.DataOwnerDeployment[i] = infra.ID
		}

		_, err = m.Collection.InsertOne(context.Background(), vdcInfo)
		if err != nil {
			log.WithError(err).Error("Error saving blueprint VDC information")
			return vdcInfo, err
		}
	} else {
		if len(vdcInfo.DataOwnerDeployment) > 0 {
			dataOwnerDeployment = make(model.DeploymentInfo, len(vdcInfo.DataOwnerDeployment))
			for i, infraID := range vdcInfo.DataOwnerDeployment {
				dataOwnerDeployment[i], err = m.DeploymentController.Repository.FindInfrastructure(infraID)
				if err != nil {
					return vdcInfo, fmt.Errorf("Error retrieving data administrator infrastructure %s: %w", infraID, err)
				}
			}
		} else {
			return vdcInfo, fmt.Errorf("Can't find data owner deployment in blueprint ID %s", bp.ID)
		}
	}

	if vdcInfo.VDMIP == "" {
		infra := m.findDefaultInfra(dataOwnerDeployment)

		vdmIP, err := m.DeployVMD(infra, vdcInfo.ID)
		if err != nil {
			return vdcInfo, fmt.Errorf("Error deploying VDM in infrastructure %s: %w", infra.ID, err)
		}
		vdcInfo.VDMIP = vdmIP
		vdcInfo.VDMInfraID = infra.ID
		_, err = m.Collection.ReplaceOne(context.Background(), bson.M{"_id": vdcInfo.ID}, vdcInfo, options.Replace())
		if err != nil {
			return vdcInfo, fmt.Errorf("Error updating VDM information: %w", err)
		}
	}

	appDeveloperDeploymentOrig := m.filterInfrastructures(ApplicationDeveloperOwnerValue, bp.CookbookAppendix.Resources.Infrastructures)

	var appDeveloperDeploymentInfo model.DeploymentInfo
	if len(appDeveloperDeploymentOrig) > 0 {
		var appDeveloperDeployment model.Deployment
		err = utils.TransformObject(appDeveloperDeploymentOrig, &appDeveloperDeployment)
		if err != nil {
			return vdcInfo, fmt.Errorf("Error transforming application developer resources: %w", err)
		}
		appDeveloperDeploymentInfo, err = m.createDeployment(appDeveloperDeployment)
		if err != nil {
			return vdcInfo, fmt.Errorf("Error creating Application Developer cluster: %w", err)
		}
	}

	totalDeployment := m.mergeDeployments(dataOwnerDeployment, appDeveloperDeploymentInfo)
	infra := m.findDefaultInfra(totalDeployment)
	if infra.ID == "" {
		return vdcInfo, errors.New("Can't find default infrastructure to deploy a new VDC")
	}

	vdcID := fmt.Sprintf("vdc-%d", vdcInfo.NumVDCs)

	bp.CookbookAppendix.Deployment, err = m.transformDeploymentInfo(bp.ID, totalDeployment)
	if err != nil {
		return vdcInfo, fmt.Errorf("Error transforming deployment information: %w", err)
	}

	vdmIP := ""
	if infra.ID != vdcInfo.VDMInfraID {
		vdmIP = vdcInfo.VDMIP
	}

	tombstonePort, cafPort, _, err := m.DeployVDC(bp, infra, vdcID, vdmIP)
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
				CAFPort:       cafPort,
			},
		},
		AppDeveloperDeployment: m.toIDs(appDeveloperDeploymentInfo),
	}

	vdcInfo.VDCs[vdcID] = config
	vdcInfo.NumVDCs++

	_, err = m.Collection.ReplaceOne(context.Background(), bson.M{"_id": vdcInfo.ID}, vdcInfo, options.Replace())
	if err != nil {
		return vdcInfo, fmt.Errorf("Error saving VDC information: %w", err)
	}

	return vdcInfo, err
}

func (m *VDCManager) findDefaultInfra(deployments ...model.DeploymentInfo) model.InfrastructureDeploymentInfo {

	var infra model.InfrastructureDeploymentInfo
	for _, deployment := range deployments {
		if deployment != nil && len(deployment) > 0 {
			for _, infra := range deployment {
				if infra.ExtraProperties.GetBool("ditas_default") {
					return infra
				}
			}
		}
	}

	return infra
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

func (m *VDCManager) getVarsFromConfig() map[string]interface{} {
	return viper.GetStringMap(DitasVariablesProperty)
}

func (m *VDCManager) doProvisionKubernetes(infra model.InfrastructureDeploymentInfo) (model.InfrastructureDeploymentInfo, error) {
	var dep model.InfrastructureDeploymentInfo

	logger := log.WithField("infrastructure", infra.ID)

	logger.Info("Waiting for SSH ports to be ready")
	err := utils.WaitForSSHReady(infra, true)
	if err != nil {
		return dep, fmt.Errorf("Error waiting for ssh port to be ready: %w", err)
	}
	logger.Info("SSH ports ready. Deploying Kubernetes")

	args := make(model.Parameters)
	dep, _, err = m.ProvisionerController.Provision(infra.ID, "kubeadm", args, "")
	//err := m.provisionKubernetesWithKubespray(deployment.ID, infra)
	if err != nil {
		return dep, utils.WrapLogAndReturnError(logger, fmt.Sprintf("Error deploying kubernetes on infrastructure %s", infra.ID), err)
	}

	dep, _, err = m.ProvisionerController.Provision(infra.ID, "helm", args, "")
	if err != nil {
		return dep, utils.WrapLogAndReturnError(logger, fmt.Sprintf("Error deploying helm in infrastructure %s", infra.ID), err)
	}

	vars := m.getVarsFromConfig()

	esURL, ok := vars[ElasticSearchUrlVarName]
	if ok {

		parsedURL, err := url.Parse(esURL.(string))
		if err != nil {
			return dep, utils.WrapLogAndReturnError(logger, fmt.Sprintf("Invalid elasticsearch URL: %s", parsedURL), err)
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

		dep, _, err = m.ProvisionerController.Provision(infra.ID, "fluentd", args, "")
		if err != nil {
			return dep, utils.WrapLogAndReturnError(logger, fmt.Sprintf("Error installing fluentd at infrastructure %s", infra.ID), err)
		}
	}

	/*dep, _, err = m.ProvisionerController.Provision(infra.ID, "rook", args, "kubernetes")
	if err != nil {
		return dep, utils.WrapLogAndReturnError(logger, fmt.Sprintf("Error deploying rook to kubernetes cluster %s", infra.ID), err)
	}*/

	return dep, err
}

func (m *VDCManager) provisionKubernetesParallel(infra model.InfrastructureDeploymentInfo, c chan KubernetesProvisionResult) {
	res, err := m.doProvisionKubernetes(infra)
	c <- KubernetesProvisionResult{
		Infra: res,
		Error: err,
	}
	return
}

func (m *VDCManager) provisionKubernetes(deployment model.DeploymentInfo) (model.DeploymentInfo, error) {
	var err error
	result := make(model.DeploymentInfo, 0)
	channel := make(chan KubernetesProvisionResult, len(deployment))

	for _, infra := range deployment {
		go m.provisionKubernetesParallel(infra, channel)
	}

	for remaining := len(deployment); remaining > 0; remaining-- {
		provisionResult := <-channel
		if provisionResult.Error != nil {
			log.WithError(err).Errorf("Error deploying kubernetes in infrastructure %s", provisionResult.Infra.ID)
			err = provisionResult.Error
		} else {
			result = append(result, provisionResult.Infra)
		}
	}

	return result, err
}

func (m *VDCManager) DeployVDC(blueprint blueprint.Blueprint, infra model.InfrastructureDeploymentInfo, vdcID, vdmIP string) (int, int, model.InfrastructureDeploymentInfo, error) {

	var deployment model.InfrastructureDeploymentInfo
	kubeConfigRaw, ok := infra.Products["kubernetes"]
	if !ok {
		return -1, -1, deployment, fmt.Errorf("Infrastructure %s doesn't have kubernetes installed", infra.ID)
	}

	var kubeconfig kubernetes.KubernetesConfiguration
	err := utils.TransformObject(kubeConfigRaw, &kubeconfig)
	if err != nil {
		return -1, -1, deployment, fmt.Errorf("Error reading kubernetes configuration from infrastructure %s: %w", infra.ID, err)
	}

	return m.doDeployVDC(infra, blueprint, vdcID, vdmIP)
}

func (m *VDCManager) doDeployVDC(infra model.InfrastructureDeploymentInfo, bp blueprint.Blueprint, vdcID, vdmIP string) (int, int, model.InfrastructureDeploymentInfo, error) {

	args := make(model.Parameters)
	args[BlueprintProperty] = bp
	args[VDCIDProperty] = vdcID
	args[VDMIPProperty] = vdmIP
	args[VariablesProperty] = m.getVarsFromConfig()

	infra, out, err := m.ProvisionerController.Provision(infra.ID, "vdc", args, "kubernetes")
	tombstonePort := -1
	cafPort := -1
	ok := false
	if err != nil {
		return tombstonePort, cafPort, infra, err
	}

	tombstonePort, ok = out.GetInt(TombstonePortProperty)
	if !ok {
		return tombstonePort, cafPort, infra, errors.New("Can't find tombstone port in deployed VDC")
	}

	cafPort, ok = out.GetInt(CAFPortProperty)
	if !ok {
		return tombstonePort, cafPort, infra, errors.New("Can't find CAF port in deployed VDC")
	}

	return tombstonePort, cafPort, infra, err
}

func (m *VDCManager) findVDCInfrastructure(vdcInfo VDCInformation, vdcID, targetInfraID string) (model.InfrastructureDeploymentInfo, error) {
	var targetInfra model.InfrastructureDeploymentInfo
	vdcConfig, ok := vdcInfo.VDCs[vdcID]
	if !ok {
		return targetInfra, fmt.Errorf("Can't find VDC with identifier %s", vdcID)
	}

	totalInfras := append(vdcInfo.DataOwnerDeployment, vdcConfig.AppDeveloperDeployment...)
	var infraID string
	for i := 0; i < len(totalInfras) && infraID == ""; i++ {
		if totalInfras[i] == targetInfraID {
			infraID = targetInfraID
		}
	}

	if infraID == "" {
		return targetInfra, fmt.Errorf("Can't find target infrastructure %s associated to blueprint %s", targetInfraID, vdcInfo.ID)
	}

	targetInfra, err := m.DeploymentController.Repository.FindInfrastructure(targetInfraID)
	if err != nil {
		return targetInfra, fmt.Errorf("Error finding target infrastructure %s: %w", targetInfraID, err)
	}

	return targetInfra, nil
}

func (m *VDCManager) CopyVDC(blueprintID, vdcID, targetInfraID string) (VDCConfiguration, error) {
	var vdcInfo VDCInformation
	var vdcConfig VDCConfiguration
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

	var bp blueprint.Blueprint
	err = json.Unmarshal([]byte(vdcConfig.Blueprint), &bp)
	if err != nil {
		return vdcConfig, fmt.Errorf("Error unmarshaling blueprint for VDC %s: %w", vdcID, err)
	}

	vdmIP := ""
	if targetInfraID != vdcInfo.VDMInfraID {
		vdmIP = vdcInfo.VDMIP
	}

	targetInfra, err := m.findVDCInfrastructure(vdcInfo, vdcID, targetInfraID)
	if err != nil {
		return vdcConfig, fmt.Errorf("Error finding target infrastructure %s: %w", targetInfraID, err)
	}

	tombstonePort, cafPort, targetInfra, err := m.DeployVDC(bp, targetInfra, vdcID, vdmIP)
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
		CAFPort:       cafPort,
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

func (m *VDCManager) DeployDatasource(blueprintId, vdcID, infraId, datasourceType string, args model.Parameters) error {
	var blueprintInfo VDCInformation
	err := m.Collection.FindOne(context.Background(), bson.M{"_id": blueprintId}).Decode(&blueprintInfo)
	if err != nil {
		return fmt.Errorf("Can't find information for blueprint %s: %s", blueprintId, err.Error())
	}

	infra, err := m.findVDCInfrastructure(blueprintInfo, vdcID, infraId)
	if err != nil {
		return fmt.Errorf("Can't finde infrastructure %s associated to blueprint %s: %v", infraId, blueprintId, err)
	}

	_, _, err = m.ProvisionerController.Provision(infra.ID, datasourceType, args, "kubernetes")
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
