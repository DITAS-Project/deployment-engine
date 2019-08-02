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

func (p *VDCManager) transformDeploymentInfo(src model.DeploymentInfo) (blueprint.DeploymentInfo, error) {
	var target blueprint.DeploymentInfo
	err := utils.TransformObject(src, &target)
	if err != nil {
		return target, err
	}

	return target, err
}

func (m *VDCManager) DeployBlueprint(bp blueprint.Blueprint) (model.DeploymentInfo, error) {
	var deploymentInfo model.DeploymentInfo
	if bp.ID == "" {
		return deploymentInfo, errors.New("Invalid blueprint. Id is mandatory")
	}

	var vdcInfo VDCInformation
	err := m.Collection.FindOne(context.Background(), bson.M{"_id": bp.ID}).Decode(&vdcInfo)
	if err != nil {

		vdcInfo = VDCInformation{
			ID:      bp.ID,
			NumVDCs: 0,
			VDCs:    make(map[string]VDCConfiguration),
		}

		var deployment model.Deployment
		err = utils.TransformObject(bp.CookbookAppendix.Resources, &deployment)

		deploymentInfo, err = m.DeploymentController.CreateDeployment(deployment)
		if err != nil {
			if deploymentInfo.ID != "" {
				errDelete := m.DeploymentController.DeleteDeployment(deploymentInfo.ID)
				if errDelete != nil {
					return deploymentInfo, fmt.Errorf("Error in deployment: %s and error cleaning deployment: %s", err.Error(), errDelete.Error())
				}
				return deploymentInfo, fmt.Errorf("Error creating deployment: %s\nPartial deployment deleted", err.Error())
			}
			return deploymentInfo, err
		}

		vdcInfo.DeploymentID = deploymentInfo.ID

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

		_, err = m.Collection.InsertOne(context.Background(), vdcInfo)
		if err != nil {
			log.WithError(err).Error("Error saving blueprint VDC information")
			return deploymentInfo, err
		}
	}

	if deploymentInfo.ID == "" {
		deploymentInfo, err = m.DeploymentController.Repository.GetDeployment(vdcInfo.DeploymentID)
		if err != nil {
			log.WithError(err).Errorf("Error finding deployment %s for blueprint %s", vdcInfo.DeploymentID, vdcInfo.ID)
			return deploymentInfo, err
		}
	}

	infra, ok := m.findDefaultInfra(deploymentInfo)
	if !ok {
		return deploymentInfo, fmt.Errorf("Can't find default infrastructure in deployment %s", deploymentInfo.ID)
	}

	vdcID := fmt.Sprintf("vdc-%d", vdcInfo.NumVDCs)

	bp.CookbookAppendix.Deployment, err = m.transformDeploymentInfo(deploymentInfo)
	if err != nil {
		return deploymentInfo, fmt.Errorf("Error transforming deployment information: %s", err.Error())
	}

	res, err := m.DeployVDC(bp, deploymentInfo, infra, vdcID)
	if err != nil {
		return res, err
	}

	strBp, err := json.Marshal(bp)
	if err != nil {
		return res, fmt.Errorf("Error marshaling blueprint: %s", err.Error())
	}

	config, ok := vdcInfo.VDCs[vdcID]
	if !ok {
		config = VDCConfiguration{
			Blueprint:       strBp,
			Infrastructures: []string{infra.ID},
		}
	} else {
		config.Infrastructures = append(config.Infrastructures, infra.ID)
	}
	vdcInfo.VDCs[vdcID] = config
	vdcInfo.NumVDCs++

	_, err = m.Collection.ReplaceOne(context.Background(), bson.M{"_id": vdcInfo.ID}, vdcInfo, options.Replace())
	if err != nil {
		return res, fmt.Errorf("Error saving VDC information: %s", err.Error())
	}

	return res, err
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

		/*result, err = m.ProvisionerController.Provision(deployment.ID, infra.ID, solution, args, "kubernetes")
		if err != nil {
			log.WithError(err).Errorf("Error deploying rook to kubernetes cluster %s of deployment %s", infra.ID, deployment.ID)
			return result, err
		}

		result, err = m.provisionPersistence("glusterfs")*/

	}
	return result, nil
}

func (m *VDCManager) DeployVDC(blueprint blueprint.Blueprint, deployment model.DeploymentInfo, infra model.InfrastructureDeploymentInfo, vdcID string) (model.DeploymentInfo, error) {

	kubeConfigRaw, ok := infra.Products["kubernetes"]
	if !ok {
		return deployment, fmt.Errorf("Infrastructure %s doesn't have kubernetes installed", infra.ID)
	}

	var kubeconfig kubernetes.KubernetesConfiguration
	err := utils.TransformObject(kubeConfigRaw, &kubeconfig)
	if err != nil {
		return deployment, fmt.Errorf("Error reading kubernetes configuration from infrastructure %s: %s", infra.ID, err.Error())
	}

	_, initialized := kubeconfig.DeploymentsConfiguration["VDM"]

	if !initialized {
		_, err = m.ProvisionerController.Provision(deployment.ID, infra.ID, "vdm", nil, "kubernetes")
		if err != nil {
			log.WithError(err).Errorf("Error initializing infrastructure %s in deployment %s to host VDCs", infra.ID, deployment.ID)
			return deployment, err
		}
	}

	return m.doDeployVDC(deployment, infra, blueprint, vdcID, "")
}

func (m *VDCManager) doDeployVDC(deployment model.DeploymentInfo, infra model.InfrastructureDeploymentInfo, bp blueprint.Blueprint, vdcID, vdmIP string) (model.DeploymentInfo, error) {

	args := make(model.Parameters)
	args["blueprint"] = bp
	args["deployment"] = deployment
	args["vdcId"] = vdcID
	args["tombstonePort"] = viper.GetInt(DitasTombstonePortProperty)
	if vdmIP != "" {
		args["vdmIP"] = vdmIP
		args["move"] = true
	}
	return m.ProvisionerController.Provision(deployment.ID, infra.ID, "vdc", args, "kubernetes")
}

func (m *VDCManager) MoveVDC(blueprintID, vdcID, targetInfraID string) (model.DeploymentInfo, error) {
	var vdcInfo VDCInformation
	var deployment model.DeploymentInfo
	err := m.Collection.FindOne(context.Background(), bson.M{"_id": blueprintID}).Decode(&vdcInfo)
	if err != nil {
		return deployment, fmt.Errorf("Error finding deployment for blueprint %s: %s", blueprintID, err.Error())
	}

	deploymentID := vdcInfo.DeploymentID
	deployment, err = m.DeploymentController.Repository.GetDeployment(deploymentID)
	if err != nil {
		return deployment, fmt.Errorf("Error gettind deployment %s associated to blueprint %s: %s", deploymentID, blueprintID, err.Error())
	}

	config, ok := vdcInfo.VDCs[vdcID]
	if !ok {
		return deployment, fmt.Errorf("Can't find VDC with identifier %s in deployment %s", vdcID, deployment.ID)
	}

	infras := config.Infrastructures

	for _, infra := range infras {
		if infra == targetInfraID {
			return deployment, fmt.Errorf("VDC with identifier %s already exists in infrastructure %s of deployment %s", vdcID, infra, deployment.ID)
		}
	}

	_, ok = deployment.Infrastructures[targetInfraID]
	if !ok {
		return deployment, fmt.Errorf("Can't find infrastructure %s in deployment %s associated to blueprint %s", targetInfraID, deployment.ID, blueprintID)
	}

	var bp blueprint.Blueprint
	err = json.Unmarshal(config.Blueprint, &bp)
	if err != nil {
		return deployment, fmt.Errorf("Error unmarshaling blueprint for VDC %s: %s", vdcID, err.Error())
	}

	vdmIP := m.findVDMIP(deployment)
	if vdmIP == "" {
		return deployment, fmt.Errorf("Can't find VDM IP in deployment %s associated to blueprint %s", deploymentID, blueprintID)
	}

	parameters := model.Parameters{
		"blueprint":  bp,
		"vdcId":      vdcID,
		"vdmIP":      vdmIP,
		"deployment": deployment,
	}

	deployment, err = m.ProvisionerController.Provision(deploymentID, targetInfraID, "vdc", parameters, "kubernetes")

	if err != nil {
		return deployment, err
	}

	config.Infrastructures = append(config.Infrastructures, targetInfraID)
	vdcInfo.VDCs[vdcID] = config

	updRes := m.Collection.FindOneAndReplace(context.Background(), bson.M{"_id": blueprintID}, vdcInfo, options.FindOneAndReplace())

	if updRes.Err() != nil {
		return deployment, fmt.Errorf("Error updating VDC information for blueprint %s: %s", blueprintID, updRes.Err())
	}

	return m.DeploymentController.Repository.SaveDeployment(deployment)
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
