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

package cloudsigma

import (
	"deployment-engine/model"
	"deployment-engine/utils"
	"errors"
	"fmt"
	"io/ioutil"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	homedir "github.com/mitchellh/go-homedir"
	"github.com/sethvargo/go-password/password"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

const (
	DeploymentType = "cloudsigma"

	BootDriveTypeProperty = "cloudsigma_boot_drive_type"

	BootDriveTypeLibrary = "library"
	BootDriveTypeCustom  = "custom"

	BootDriveTypeDefault = BootDriveTypeLibrary
)

type CloudsigmaDeployer struct {
	publicKey string
	client    *Client
}

type NodeCreationResult struct {
	Info  model.NodeInfo
	Error error
}

type DiskCreationResult struct {
	Disk  ResourceType
	Error error
}

type HostDisks struct {
	Drive ResourceType
	Data  []ResourceType
}

func NewDeployer(apiURL string, credentials model.BasicAuthSecret) (*CloudsigmaDeployer, error) {
	home, err := homedir.Dir()
	if err != nil {
		log.Infof("Error getting home folder: %s\n", err.Error())
		return nil, err
	}
	viper.SetDefault("debug_cs_client", false)
	err = viper.ReadInConfig()
	if err == nil {

		pubKeyRaw, err := ioutil.ReadFile(home + "/.ssh/id_rsa.pub")
		if err == nil {
			pubKey := string(pubKeyRaw)
			client := NewClient(apiURL,
				credentials.Username, credentials.Password, viper.GetBool("debug_cs_client"))
			return &CloudsigmaDeployer{
				client:    client,
				publicKey: pubKey,
			}, nil
		}

		log.Infof("Error reading public key: %s\n", err.Error())

	} else {
		log.Infof("Error reading configuration: %s\n", err.Error())
	}
	return nil, err
}

func (d *CloudsigmaDeployer) returnError(logger *log.Entry, msg string, result NodeCreationResult, err error, c chan NodeCreationResult) error {
	logger.Errorf(msg)
	result.Error = err
	deleteError := d.deletePartialDeployment(logger, result)
	if deleteError != nil {
		result.Error = deleteError
	}
	c <- result
	return err
}

func (d *CloudsigmaDeployer) waitForDiskReady(logInput *log.Entry, uuid, status string) DiskCreationResult {
	logger := logInput.WithField("drive", uuid)
	timeout := 60 * time.Second
	drive, timedOut, err := d.waitForStatusChange(uuid, status, timeout, d.client.GetDriveDetails)
	result := DiskCreationResult{
		Disk:  drive,
		Error: err,
	}

	if err != nil {
		logger.WithError(err).Error("Error waiting for disk to be ready")
		return result
	}

	if timedOut {
		result.Error = errors.New("Timeout waiting for disk to be ready")
		logger.WithError(result.Error).Error("Error waiting for disk to be ready")
		return result
	}

	if drive.Status != "unmounted" {
		result.Error = fmt.Errorf("Drive in unexpected state: %s", drive.Status)
		logger.WithError(result.Error).Error("Disk in unexpected state")
		return result
	}

	logger.Infof("Disk Ready!")
	return result
}

func (d *CloudsigmaDeployer) createDataDisk(logInput *log.Entry, hostname string, storage model.Drive, c chan DiskCreationResult) {
	logInput.Info("Creating data disk")
	dataDisk, err := d.client.CreateDrive(ResourceType{
		Media: "disk",
		Size:  storage.Size * 1024 * 1024,
		Name:  fmt.Sprintf("data-%s-%s", hostname, storage.Name),
	})
	result := DiskCreationResult{
		Disk:  dataDisk,
		Error: err,
	}
	if err != nil {
		logInput.WithError(err).Error("Error creating data disk")
		c <- result
		return
	}
	logInput.Info("Data disk created")
	logger := logInput.WithField("disk", dataDisk.UUID)
	logger.Info("Waiting for data disk to be ready")
	c <- d.waitForDiskReady(logger, dataDisk.UUID, "creating")
	return
}

func (d *CloudsigmaDeployer) isLibraryDrive(resource model.ResourceType) bool {
	return resource.ExtraProperties == nil || resource.ExtraProperties[BootDriveTypeProperty] == "" || resource.ExtraProperties[BootDriveTypeProperty] == BootDriveTypeLibrary
}

func (d *CloudsigmaDeployer) cloneDisk(logInput *log.Entry, hostname string, resource model.ResourceType, c chan DiskCreationResult) {

	logger := log.WithField("disk", resource.ImageId)
	drive := ResourceType{}

	var err error
	if resource.Disk != 0 {
		drive.Size = resource.Disk * 1024 * 1024
	}

	drive.Name = fmt.Sprintf("boot-%s", hostname)

	logger.Info("Cloning disk")

	cloned, err := d.client.CloneDrive(resource.ImageId, &drive, d.isLibraryDrive(resource))

	result := DiskCreationResult{
		Disk:  cloned,
		Error: err,
	}

	if err != nil {
		logger.WithError(err).Error("Error cloning disk")
		c <- result
		return
	}

	if cloned.UUID == "" {
		result.Error = fmt.Errorf("Empty UUID found for cloned drive of %s", drive.UUID)
		c <- result
		return
	}

	logger.Info("Disk cloned. Waiting for it to be ready...")
	c <- d.waitForDiskReady(logger, cloned.UUID, "cloning_dst")
	return
}

func (d *CloudsigmaDeployer) createHostDrives(logInput *log.Entry, hostname string, resource model.ResourceType) (HostDisks, error) {
	logInput.Info("Creating host drives")
	result := HostDisks{
		Data: make([]ResourceType, 0, len(resource.Drives)),
	}
	totalDisks := len(resource.Drives) + 1
	c := make(chan DiskCreationResult, totalDisks)
	go d.cloneDisk(logInput, hostname, resource, c)
	for _, localDisk := range resource.Drives {
		go d.createDataDisk(logInput, hostname, localDisk, c)
	}
	var err error
	for remaining := totalDisks; remaining > 0; remaining-- {
		drive := <-c
		if drive.Error != nil {
			err = drive.Error
		}

		if drive.Disk.Name != "" && drive.Disk.UUID != "" {
			if strings.HasPrefix(drive.Disk.Name, "boot-") {
				result.Drive = drive.Disk
			} else {
				result.Data = append(result.Data, drive.Disk)
			}
		} else {
			logInput.Error("Found drive without ID and name")
		}

	}
	if err == nil {
		log.Info("Host drives successfully created")
	}
	return result, err
}

func (d *CloudsigmaDeployer) createServer(logger *log.Entry, nodeName string, resource model.ResourceType, disk ResourceType, dataDisks []ResourceType, ip IPReferenceType, pw string) (ResourceType, error) {
	conf := "static"

	drives := make([]ServerDriveType, len(dataDisks)+1)

	drives[0] = ServerDriveType{
		BootOrder:  1,
		DevChannel: "0:0",
		Device:     "virtio",
		Drive:      disk}

	for i, drive := range dataDisks {
		drives[i+1] = ServerDriveType{
			BootOrder:  i + 2,
			DevChannel: fmt.Sprintf("0:%d", i+1),
			Device:     "virtio",
			Drive:      drive}
	}

	servers := RequestResponseType{
		Objects: []ResourceType{ResourceType{
			Name:        nodeName,
			CPU:         resource.CPU,
			Mem:         resource.RAM * 1024 * 1024,
			VNCPassword: pw,
			Drives:      drives,
			NICS: []ServerNICType{ServerNICType{
				IPV4Conf: ServerIPV4ConfType{
					Conf: conf,
					IP:   ip,
				},
				Model: "virtio",
			}},
			Meta: map[string]string{
				"ssh_public_key": d.publicKey,
			},
			SMP: resource.Cores,
		}},
	}

	servers, err := d.client.CreateServers(servers)

	if err != nil {
		logger.WithError(err).Error("Error creating server")
		return ResourceType{}, err
	}

	if len(servers.Objects) == 0 {
		err = errors.New("The server could not be created but we didn't get an error")
		logger.WithError(err).Error("Error creating server")
		return ResourceType{}, err
	}

	server := servers.Objects[0]

	if server.UUID == "" {
		err = errors.New("Server created without an UUID")
		logger.WithError(err).Error("Error creating server")
		return server, err
	}

	logger.Info("Server created!")
	return server, nil
}

func (d *CloudsigmaDeployer) startServer(logger *log.Entry, uuid string) (ResourceType, error) {
	logger.Info("Starting server")
	logger.WithField("server", uuid)
	actionResult, err := d.client.ExecuteServerAction(uuid, ServerStartAction)
	if err != nil {
		logger.WithError(err).Error("Error starting server")
		return ResourceType{}, err
	}

	if actionResult.Result != "success" {
		err := fmt.Errorf("Unexpected state of start operation for server %s: %s", uuid, actionResult.Result)
		logger.WithError(err).Error("Unexpected state for server")
		return ResourceType{}, err
	}
	logger.Info("Server booting")

	timeout := 120 * time.Second
	server, timedOut, err := d.waitForStatusChange(uuid, "starting", timeout, d.client.GetServerDetails)

	logger.Infof("Waiting for server to start")

	if err != nil {
		logger.WithError(err).Error("Error waiting for server to be ready")
		return server, err
	}

	if timedOut {
		err := fmt.Errorf("Timeout waiting for server %s to be ready", server.UUID)
		logger.WithError(err).Error("Error waiting for server to be ready")
		return server, err
	}

	logger.Info("Server started!!")

	return server, nil
}

func (d *CloudsigmaDeployer) CreateServer(resource model.ResourceType, ip IPReferenceType, pfx string, c chan NodeCreationResult) error {
	result := NodeCreationResult{}

	logger := log.WithField("resource", resource.Name)

	if resource.Name == "" {
		return d.returnError(logger, "", result, fmt.Errorf("Resource with empty name found in infrastructure "+pfx), c)
	}

	nodeName, err := d.clearHostName(pfx + "-" + resource.Name)
	if err != nil {
		return d.returnError(logger, fmt.Sprintf("Invalid combination of hostname for infrastructure %s and resource %s", pfx, resource.Name), result, err, c)
	}

	result = NodeCreationResult{
		Info: model.NodeInfo{
			Role:            strings.ToLower(resource.Role),
			Hostname:        nodeName,
			Username:        "cloudsigma",
			ExtraProperties: resource.ExtraProperties,
		},
	}
	logger.Info("Creating node", nodeName)

	pw, err := password.Generate(10, 3, 2, false, false)

	if err != nil {
		return d.returnError(logger, fmt.Sprintf("Error generating random password: %s\n", err.Error()), result, err, c)
	}

	disks, err := d.createHostDrives(logger, nodeName, resource)
	result.Info.DriveUUID = disks.Drive.UUID
	result.Info.DataDrives = make([]model.DriveInfo, len(disks.Data))
	result.Info.DriveSize = disks.Drive.Size
	for i, disk := range disks.Data {
		result.Info.DataDrives[i] = model.DriveInfo{
			Name: disk.Name,
			UUID: disk.UUID,
			Size: disk.Size,
		}
	}

	if err != nil {
		return d.returnError(logger, "Error creating disks", result, err, c)
	}

	logger.Infof("Creating server")

	server, err := d.createServer(logger, nodeName, resource, disks.Drive, disks.Data, ip, pw)
	result.Info.UUID = server.UUID
	if err != nil {
		return d.returnError(logger, "Error creating server", result, err, c)
	}

	server, err = d.startServer(logger, server.UUID)
	if err != nil {
		return d.returnError(logger, "Error starting server", result, err, c)
	}

	if len(server.NICS) == 0 {
		msg := "Can't find network information for server"
		return d.returnError(logger, msg, result, errors.New(msg), c)
	}

	result.Info.IP = ip.UUID
	result.Info.CPU = server.CPU
	result.Info.RAM = server.Mem
	result.Info.Cores = server.SMP

	logger.Info("Server deployment complete!!!!")

	c <- result
	return nil
}

func (d *CloudsigmaDeployer) waitForStatusChange(uuid string, status string, timeout time.Duration, getter func(string) (ResourceType, error)) (ResourceType, bool, error) {
	var resource ResourceType
	var err error
	_, timedOut, err := utils.WaitForStatusChange(status, timeout, func() (string, error) {
		resource, err = getter(uuid)
		return resource.Status, err
	})
	return resource, timedOut, err
}

func (d *CloudsigmaDeployer) deletePartialDeployment(logInput *log.Entry, nodeInfo NodeCreationResult) error {
	logInput.Info("Undoing partial deployment...")
	return d.deleteHost(logInput, nodeInfo.Info)
}

func (d *CloudsigmaDeployer) findFreeIps(log *log.Entry, numNodes int) ([]IPReferenceType, error) {
	ipInfo, err := d.client.GetAvailableIps()
	result := make([]IPReferenceType, 0, numNodes)
	if err == nil {
		numIps := len(ipInfo.Objects)
		for i := 0; i < numIps && len(result) < numNodes; i++ {
			currentIP := ipInfo.Objects[i]
			if currentIP.Server == nil {
				logger := log.WithField("IP", currentIP.UUID)
				ref, err := d.client.GetIPReference(currentIP.UUID)
				if err == nil {
					result = append(result, ref)
				} else {
					logger.WithError(err).Error("Error getting IP information")
				}
			}
		}
	} else {
		log.WithError(err).Error("Error getting list of free IPs")
	}
	return result, err
}

func (d CloudsigmaDeployer) clearHostName(hostname string) (string, error) {
	toReplace := strings.ToLower(hostname)
	reg, err := regexp.Compile("[^a-zA-Z0-9-]+")
	if err != nil {
		return "", err
	}

	replaced := reg.ReplaceAllString(toReplace, "")
	if len(replaced) > 255 || len(replaced) == 0 {
		return "", fmt.Errorf("Infrastructure or host name of resource is too long or too short. Infrastructure name + resource name should be between 0 and 255 alphanumeric characters")
	}

	return replaced, nil
}

func (d CloudsigmaDeployer) DeployInfrastructure(deploymentID string, infra model.InfrastructureType) (model.InfrastructureDeploymentInfo, error) {

	deployment := model.InfrastructureDeploymentInfo{
		ID:              uuid.New().String(),
		Products:        make(map[string]interface{}),
		ExtraProperties: infra.ExtraProperties,
	}

	if infra.Name == "" {
		return deployment, errors.New("Name is mandatory for each cloudsigma infrastructure")
	}

	numNodes := len(infra.Resources)
	deployment.Name = infra.Name
	deployment.Type = DeploymentType
	deployment.Nodes = make(map[string][]model.NodeInfo)
	deployment.Status = "creating"

	var logger = log.WithField("deployment", infra.Name)

	logger.Info("Getting the list of free IPs")
	ips, err := d.findFreeIps(logger, numNodes)
	if err != nil {
		return deployment, err
	}

	if len(ips) < numNodes {
		return deployment, errors.New("Not enough free IPs found to create the cluster")
	}

	logger.Infof("Found enough free IPs")

	c := make(chan NodeCreationResult, numNodes)

	for i, resource := range infra.Resources {
		go d.CreateServer(resource, ips[i], infra.Name, c)
	}

	var failed = false

	for remaining := numNodes; remaining > 0; remaining-- {
		result := <-c
		if result.Error == nil {
			role := strings.ToLower(result.Info.Role)
			roleNodes, ok := deployment.Nodes[role]
			if !ok {
				roleNodes = make([]model.NodeInfo, 0, 1)
			}
			roleNodes = append(roleNodes, result.Info)
			deployment.Nodes[role] = roleNodes
		} else {
			failed = true
		}
	}

	if failed {
		logger.Errorf("Deployment failed")
		deployment.Status = "failed"
		return deployment, errors.New("Deployment failed")
	}
	logger.Infof("Nodes successfully created")
	deployment.Status = "running"

	return deployment, nil
}

func (d *CloudsigmaDeployer) deleteDrive(logInput *log.Entry, uuid string) error {
	logger := logInput.WithField("drive", uuid)
	logger.Info("Deleting drive from host")
	err := d.client.DeleteDrive(uuid)
	if err != nil {
		logger.WithError(err).Error("Error deleting drive")
		return err
	}
	logger.Info("Drive deleted")
	return nil
}

func (d *CloudsigmaDeployer) deleteHost(logInput *log.Entry, host model.NodeInfo) error {

	logger := log.WithField("host", host.Hostname)
	if host.UUID != "" {
		serverInfo, err := d.client.GetServerDetails(host.UUID)
		status := "running"
		if err != nil {
			logger.WithError(err).Error("Error getting host status. Let's suppose it's running")
		} else {
			status = serverInfo.Status
		}

		if status == "running" {
			logger.Info("Stopping server")
			stopResult, err := d.client.ExecuteServerAction(host.UUID, ServerStopAction)
			if err != nil {
				log.WithError(err).Error("Error issuing stop action")
				return err
			}

			if stopResult.Result != "success" {
				msg := "Stop action was unsuccessful"
				log.Errorf(msg)
				return errors.New(msg)
			}

			logger.Info("Waiting for server to stop")
			server, timedOut, err := d.waitForStatusChange(host.UUID, "stopping", 60*time.Second, func(uuid string) (ResourceType, error) {
				return d.client.GetServerDetails(uuid)
			})

			if err != nil {
				logger.WithError(err).Error("Error stopping server")
				return err
			}

			if timedOut {
				msg := "Timeout while waiting for server to stop"
				logger.Error(msg)
				return errors.New(msg)
			}

			if server.Status != "stopped" {
				msg := fmt.Sprintf("Invalid server status. Expected 'stopped' but found '%s'", server.Status)
				logger.Error(msg)
				return errors.New(msg)
			}
			logger.Info("Server stopped")
		}
		logger.Info("Deleting server")
		err = d.client.DeleteServerWithDrives(host.UUID)

		if err != nil {
			logger.WithError(err).Error("Error deleting server with drives")
			return err
		}
		logger.Info("Host successfully deleted")
		return nil
	}

	var err error
	if len(host.DataDrives) > 0 {

		for _, drive := range host.DataDrives {
			logger := logger.WithField("drive", drive.UUID)
			err2 := d.deleteDrive(logger, drive.UUID)
			if err2 != nil {
				logger.WithError(err2).Error("Error deleting drive")
				err = err2
			}
		}
	}

	if host.DriveUUID != "" {
		logger := logger.WithField("drive", host.DriveUUID)
		err2 := d.deleteDrive(logger, host.DriveUUID)
		if err2 != nil {
			logger.WithError(err2).Error("Error deleting drive")
			err = err2
		}
	}

	return err

}

func (d CloudsigmaDeployer) DeleteInfrastructure(deploymentID string, infra model.InfrastructureDeploymentInfo) map[string]error {
	logger := log.WithField("infrastructure", infra.ID)

	logger.Info("Deleting infrastructure")

	result := make(map[string]error)

	logger.Info("Deleting nodes")
	infra.ForEachNode(func(node model.NodeInfo) {
		err := d.deleteHost(logger, node)
		if err != nil {
			logger.WithError(err).Errorf("Error deleting node %s", node.Hostname)
			result[node.Hostname] = err
		}
	})

	if len(result) == 0 {
		logger.Info("Nodes deleted. Infrastructure clear")
	}

	return result
}
