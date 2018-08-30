// model.go

package main

import (
	"deployment-engine/src/cloudsigma"
	"deployment-engine/src/ditas"
	"deployment-engine/src/utils"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	blueprint "github.com/DITAS-Project/blueprint-go"
	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"

	log "github.com/sirupsen/logrus"
)

const (
	errorStatus         = "error"
	runningStatus       = "running"
	deletingStatus      = "deleting"
	componentNameRegexp = "[a-z]([-a-z0-9]*[a-z0-9])?"
)

type DeploymentEngineController struct {
	collection *mgo.Collection
	homedir    string
}

func sanitize(name string) (string, error) {
	replaced := strings.Replace(name, "_", "-", -1)
	replaced = strings.Replace(replaced, " ", "-", -1)
	replaced = strings.ToLower(replaced)
	valid, err := regexp.Match(componentNameRegexp, []byte(replaced))
	if err != nil {
		return "", err
	}
	if !valid {
		return "", fmt.Errorf("Sanitized blueprint name %s is not valid for kubernetes deployment", replaced)
	}
	return replaced, err
}

func (c *DeploymentEngineController) deleteDeployment(log *log.Entry, bpName string, deployment ditas.InfrastructureDeployment) error {

	c.setInfraStatus(bpName, deployment.ID, "deleting")
	if deployment.Type == cloudsigma.DeploymentType {
		depLogger := log.WithField("infrastructure", deployment.ID)
		depLogger.Info("Deleting infrastructure")
		deployer, err := cloudsigma.NewDeployer()
		if err != nil {
			depLogger.WithError(err).Error("Error getting deployer")
			return err
		}
		errMap := deployer.DeleteInfrastructure(deployment, bpName)
		if len(errMap) > 0 {
			errMsg := "Error deleting infrastructure"
			c.setInfraStatus(bpName, deployment.ID, "error")
			return errors.New(errMsg)
		}
		c.removeInfra(bpName, deployment.ID)
	}

	return nil
}

func (c *DeploymentEngineController) DeleteVDC(bpName string, vdcId string, deleteDeployment bool) error {
	logger := log.WithField("blueprint", bpName)

	bpNameSanitized, err := sanitize(bpName)

	if err != nil {
		logger.Errorf("Error sanitizing blueprint name: %s", err.Error())
		return err
	}

	if vdcId != "" {
		logger = logger.WithField("VDC", vdcId)
	}

	deployment, err := c.findDeployment(bpName)
	if err == nil {
		if vdcId != "" {
			//TODO: Remove VDC
		}

		for _, infra := range deployment.Infrastructures {
			if deleteDeployment && (vdcId == "" || len(infra.VDCs) == 0) {
				err := c.deleteDeployment(logger, bpName, infra)
				if err != nil {
					logger.WithError(err).Error("Error deleting infrastructure")
					return err
				}
			}
		}

		c.collection.RemoveId(bpName)

		err = os.RemoveAll("kubernetes/" + bpNameSanitized)
		if err != nil {
			logger.WithError(err).Error("Error cleaning blueprint folder")
			return err
		}

		logger.Info("Deployment successfully deleted")

		return nil
	}
	return err
}

func (c *DeploymentEngineController) updateDeployment(bpName string, update bson.M) {
	err := c.collection.UpdateId(bpName, update)
	if err != nil {
		fmt.Printf("Error updating blueprint %s status to %v: %s", bpName, update, err.Error())
	}
}

func (c *DeploymentEngineController) removeInfra(bpName, infraId string) {
	c.updateDeployment(bpName, bson.M{
		"$pull": bson.M{"infrastructures": bson.M{"id": infraId}},
	})
}

func (c *DeploymentEngineController) setInfraStatus(bpName, infraId string, status string) {
	c.collection.UpdateWithArrayFilters(
		bson.M{"_id": bpName},
		bson.M{"$set": bson.M{"infrastructures.$[infra].status": status}},
		[]bson.M{bson.M{"infra.id": infraId}},
		false)
}

func (c *DeploymentEngineController) setGlobalStatus(bpName string, status string) {
	c.updateDeployment(bpName, bson.M{"$set": bson.M{"status": status}})
}

func (c *DeploymentEngineController) addVdcToInfra(bpName, infraId, vdcId string, bp blueprint.BlueprintType) {
	c.collection.UpdateWithArrayFilters(
		bson.M{"_id": bpName},
		bson.M{
			"$inc": bson.M{"infrastructures.$[infra].num_vdcs": 1},
			"$set": bson.M{"infrastructures.$[infra].vdcs." + vdcId: bp}},
		[]bson.M{bson.M{"infra.id": infraId}},
		false)
}

func (c *DeploymentEngineController) findDeployment(bpName string) (ditas.Deployment, error) {
	var deployment ditas.Deployment
	err := c.collection.FindId(bpName).One(&deployment)
	return deployment, err
}

func writeHost(node ditas.NodeInfo, file *os.File) (int, error) {
	line := fmt.Sprintf("%s ansible_host=%s ansible_user=%s\n", node.Name, node.IP, node.Username)
	return file.WriteString(line)
}

func (c *DeploymentEngineController) createInventory(logger *log.Entry, bpID string, deployment ditas.InfrastructureDeployment) error {
	path := "kubernetes/" + bpID

	err := os.MkdirAll(path, os.ModePerm)
	if err != nil {
		logger.WithError(err).Errorf("Error creating inventory folder %s", path)
		return err
	}

	filePath := path + "/inventory"
	logger.Infof("Creating inventory at %s", filePath)
	inventory, err := os.Create(filePath)
	defer inventory.Close()

	if err != nil {
		logger.WithError(err).Errorf("Error creating inventory file %s", filePath)
		return err
	}

	_, err = inventory.WriteString("[master]\n")
	if err != nil {
		logger.WithError(err).Error("Error writing master header to inventory")
		return err
	}

	_, err = writeHost(deployment.Master, inventory)
	if err != nil {
		logger.WithError(err).Error("Error writing master information to inventory")
		return err
	}

	_, err = inventory.WriteString("[slaves]\n")
	if err != nil {
		logger.WithError(err).Error("Error writing slaves header to inventory")
		return err
	}
	for _, slave := range deployment.Slaves {
		_, err = writeHost(slave, inventory)
		if err != nil {
			logger.WithError(err).Errorf("Error writing slave %s header to inventory", slave.Name)
			return err
		}
	}

	logger.Info("Inventory correctly created")

	return nil
}

func (c *DeploymentEngineController) writeBlueprint(logger *log.Entry, bp blueprint.BlueprintType, bpID, vdcId string, infra ditas.InfrastructureDeployment) error {
	path := "kubernetes/" + bpID + "/" + infra.ID + "/" + vdcId

	err := os.MkdirAll(path, os.ModePerm)
	if err != nil {
		logger.WithError(err).Errorf("Error creating infrastructure blueprints folder %s", path)
		return err
	}

	name := path + "/blueprint.json"
	logger.Infof("Copying blueprint to %s", name)

	jsonData, err := json.Marshal(bp)
	jsonFile, err := os.Create(name)
	if err != nil {
		logger.WithError(err).Errorf("Error creating blueprint file %s", name)
		return err
	}
	defer jsonFile.Close()
	_, err = jsonFile.Write(jsonData)
	if err != nil {
		logger.WithError(err).Errorf("Error writing blueprint file %s", name)
		return err
	}

	logger.Info("Blueprint copied")

	return nil
}

func (c *DeploymentEngineController) addVDM(logger *log.Entry, bpId string) error {
	logger.Info("Adding VDM")

	inventory := fmt.Sprintf("--inventory=kubernetes/%s/inventory", bpId)
	err := utils.ExecuteCommand(logger, "ansible-playbook", "kubernetes/ansible_deploy_vdm.yml", inventory)

	if err != nil {
		logger.WithError(err).Error("Error adding VDM")
		return err
	}

	logger.Info("VDM added")
	return nil
}

func (c *DeploymentEngineController) deployK8s(logger *log.Entry, bpId string, deployment ditas.InfrastructureDeployment) error {
	err := c.createInventory(logger, bpId, deployment)
	if err != nil {
		return err
	}

	logger.Info("Calling Ansible for initial k8s deployment")
	//time.Sleep(180 * time.Second)
	vars := fmt.Sprintf("masterUsername=%s", deployment.Master.Username)
	inventory := fmt.Sprintf("--inventory=kubernetes/%s/inventory", bpId)
	err = utils.ExecuteCommand(logger, "ansible-playbook", "kubernetes/ansible_deploy.yml", inventory, "--extra-vars", vars)

	if err != nil {
		logger.WithError(err).Error("Error executing ansible deployment for k8s deployment")
		return err
	}

	logger.Info("K8s cluster created")
	return nil
}

func (c *DeploymentEngineController) addHostToHostFile(log *log.Entry, hostInfo ditas.NodeInfo) error {
	logger := log.WithField("host", hostInfo.Name)
	host := fmt.Sprintf("%s@%s", hostInfo.Username, hostInfo.IP)
	command := fmt.Sprintf("echo %s %s | sudo tee -a /etc/hosts > /dev/null 2>&1", hostInfo.IP, hostInfo.Name)
	timeout := 30 * time.Second
	logger.Info("Waiting for ssh service to be ready")
	_, timedOut, _ := utils.WaitForStatusChange("starting", timeout, func() (string, error) {
		err := utils.ExecuteCommand(logger, "ssh", "-o", "StrictHostKeyChecking=no", host, command)
		if err != nil {
			return "starting", nil
		}
		return "started", nil
	})
	if timedOut {
		msg := "Timeout waiting for ssh service to start"
		logger.Errorf(msg)
		return errors.New(msg)
	}
	logger.Info("Ssh service ready")

	return nil
}

func (c *DeploymentEngineController) addToHostFile(logger *log.Entry, infra ditas.InfrastructureDeployment) error {

	logger.Info("Adding master to hosts")
	err := c.addHostToHostFile(logger, infra.Master)

	if err != nil {
		logger.WithError(err).Error("Error adding master to hosts")
		return err
	}

	logger.Info("Master added. Adding slaves to hosts")

	for _, slave := range infra.Slaves {
		err = c.addHostToHostFile(logger, slave)
		if err != nil {
			logger.WithError(err).Errorf("Error adding slave %s to hosts", slave.Name)
			return err
		}
	}

	logger.Info("Slaves added")

	return nil
}

func (c *DeploymentEngineController) deployVdc(log *log.Entry, bpId, vdcID string, deployment ditas.InfrastructureDeployment) error {

	logger := log.WithField("deployment", deployment.ID).WithField("VDC", vdcID)
	logger.Infof("Deploying VDC")
	//time.Sleep(180 * time.Second)
	vars := fmt.Sprintf("vdcId=%s blueprintId=%s infraId=%s", vdcID, bpId, deployment.ID)
	inventory := fmt.Sprintf("--inventory=kubernetes/%s/inventory", bpId)
	err2 := utils.ExecuteCommand(logger, "ansible-playbook", "kubernetes/ansible_deploy_add.yml", inventory, "--extra-vars", vars)

	if err2 != nil {
		logger.WithError(err2).Error("Error adding VDC")
		return err2
	}
	logger.Info("VDC added")

	return nil
}

func (c *DeploymentEngineController) CreateDep(bp blueprint.BlueprintType) error {

	bpName := *bp.InternalStructure.Overview.Name
	logger := log.WithField("blueprint", bpName)

	logger.Info("Starting deployment of a new VDC")

	bpNameSanitized, err := sanitize(bpName)

	if err != nil {
		logger.Errorf("Error sanitizing blueprint name: %s", err.Error())
		return err
	}

	var deployment ditas.Deployment
	c.collection.FindId(bpName).One(&deployment)
	if err != nil || deployment.ID == "" {

		logger.Info("Infrastructure not found. Creating a Kubernetes cluster to host the VDC and VDM")

		deployment = ditas.Deployment{
			ID:              bpName,
			Infrastructures: make([]ditas.InfrastructureDeployment, len(bp.CookbookAppendix.Infrastructure)),
			Status:          "starting",
		}
		err = c.collection.Insert(deployment)
		if err != nil {
			logger.WithError(err).Error("Error inserting deployment in the database")
			return err
		}

		var deployer ditas.Deployer
		for i, infra := range bp.CookbookAppendix.Infrastructure {
			if strings.ToLower(infra.APIType) == "cloudsigma" {
				deployer, err = cloudsigma.NewDeployer()
				if err != nil {
					fmt.Printf("Error creating cloudsigma deployer: %s\n", err.Error())
					return err
				}
				infraDeployment, err := deployer.DeployInfrastructure(infra, bpNameSanitized)
				if err == nil {
					deployment.Infrastructures[i] = infraDeployment
					err = c.collection.UpdateId(deployment.ID, deployment)
					if err != nil {
						logger.WithError(err).Error("Error updating deployment status")
					}

					err = c.addToHostFile(logger, infraDeployment)
					if err != nil {
						logger.WithError(err).Error("SSH is not available")
						return err
					}

					err = c.deployK8s(logger, bpNameSanitized, infraDeployment)
					if err != nil {
						logger.WithError(err).Error("Error deploying kubernetes cluster")
						return err
					}

					err = c.addVDM(logger, bpNameSanitized)
					if err != nil {
						logger.WithError(err).Error("Error adding VDM")
						return err
					}

					c.setInfraStatus(bpName, infraDeployment.ID, "running")
				} else {
					c.collection.RemoveId(bpName)
					return err
				}
			}
		}

		c.setGlobalStatus(bpName, "running")
	}

	for _, infra := range deployment.Infrastructures {

		vdcId := "vdc-" + strconv.Itoa(infra.NumVDCs)

		bp.InternalStructure.Overview.Name = &vdcId

		err = c.writeBlueprint(logger, bp, bpNameSanitized, vdcId, infra)
		if err != nil {
			logger.WithError(err).Error("Error writing blueprint")
			return err
		}

		err = c.deployVdc(logger, bpNameSanitized, vdcId, infra)
		if err != nil {
			logger.WithError(err).Error("Error adding VDC")
			return err
		}

		c.addVdcToInfra(bpName, infra.ID, vdcId, bp)

		logger.Info("VDC deployment finished")
	}

	return nil
}

func (c *DeploymentEngineController) GetAllDeps() ([]ditas.Deployment, error) {
	var result []ditas.Deployment
	err := c.collection.Find(nil).All(&result)
	return result, err
}

func (c *DeploymentEngineController) GetDep(id string) (ditas.Deployment, error) {
	return c.findDeployment(id)
}
