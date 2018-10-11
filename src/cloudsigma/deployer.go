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

package cloudsigma

import (
	"deployment-engine/src/ditas"
	"deployment-engine/src/utils"
	"errors"
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"
	"time"

	blueprint "github.com/DITAS-Project/blueprint-go"

	homedir "github.com/mitchellh/go-homedir"
	"github.com/sethvargo/go-password/password"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

const (
	DeploymentType = "cloudsigma"
)

type CloudsigmaDeployer struct {
	publicKey string
	client    *Client
}

type NodeCreationResult struct {
	Info  ditas.NodeInfo
	Error error
}

type DiskCreationResult struct {
	Disk  ResourceType
	Error error
}

type HostDisks struct {
	Drive ResourceType
	Data  ResourceType
}

func NewDeployer() (*CloudsigmaDeployer, error) {
	home, err := homedir.Dir()
	if err != nil {
		log.Infof("Error getting home folder: %s\n", err.Error())
		return nil, err
	}
	viper.SetDefault("debug_cs_client", false)
	viper.SetConfigType("properties")
	viper.SetConfigFile(home + "/.cloudsigma.conf")
	err = viper.ReadInConfig()
	if err == nil {

		pubKeyRaw, err := ioutil.ReadFile(home + "/.ssh/id_rsa.pub")
		if err == nil {
			pubKey := string(pubKeyRaw)
			client := NewClient(viper.GetString("api_endpoint"),
				viper.GetString("username"), viper.GetString("password"), viper.GetBool("debug_cs_client"))
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
	deleteError := d.deletePartialDeployment(result)
	if deleteError != nil {
		result.Error = deleteError
	}
	c <- result
	return err
}

func (d *CloudsigmaDeployer) createDataDisk(logInput *log.Entry, nodeName string, c chan DiskCreationResult) {
	logInput.Info("Creating data disk")
	dataDisk, err := d.client.CreateDrive(ResourceType{
		Media: "disk",
		Size:  5 * 1024 * 1024 * 1024,
		Name:  fmt.Sprintf("data-%s", nodeName),
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

func (d *CloudsigmaDeployer) cloneDisk(logInput *log.Entry, resource blueprint.ResourceType, c chan DiskCreationResult) {

	params := map[string]string{
		"distribution": resource.OS,
		"version":      resource.BaseImage,
	}
	logger := logInput.WithField("disk", resource.OS+" "+resource.BaseImage)
	logger.Infof("Finding disk image of OS %s and version %s\n", resource.OS, resource.BaseImage)
	drive, err := d.client.GetLibDrive(params)

	if err != nil {
		logger.WithError(err).Error("Error finding disk image")
		c <- DiskCreationResult{
			Error: err,
		}
		return
	}

	if drive.UUID == "" {
		err := fmt.Errorf("Empty UUID for drive when looking for OS %s and version %s", resource.OS, resource.BaseImage)
		logger.WithError(err).Print("")
		c <- DiskCreationResult{
			Error: err,
		}
		return
	}

	if resource.Disk != "" {
		drive.Size, err = strconv.Atoi(resource.Disk)
		if err != nil {
			log.WithError(err).Error("Error setting size. Default size will be used")
		}
	}

	drive.Name = fmt.Sprintf("drive-%s-%s", drive.Name, resource.Name)

	logger.Infof("Image found. Cloning disk %s\n", drive.UUID)

	cloned, err := d.client.CloneDrive(drive.UUID, &drive)

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

func (d *CloudsigmaDeployer) createHostDrives(logInput *log.Entry, resource blueprint.ResourceType) (HostDisks, error) {
	logInput.Info("Creating host drives")
	var result HostDisks
	c := make(chan DiskCreationResult, 2)
	go d.cloneDisk(logInput, resource, c)
	go d.createDataDisk(logInput, resource.Name, c)
	var err error
	for remaining := 2; remaining > 0; remaining-- {
		drive := <-c
		if drive.Error != nil {
			err = drive.Error
		}

		if drive.Disk.Name != "" && drive.Disk.UUID != "" && strings.HasPrefix(drive.Disk.Name, "data-") {
			result.Data = drive.Disk
		} else {
			result.Drive = drive.Disk
		}

	}
	if err == nil {
		log.Info("Host drives successfully created")
	}
	return result, err
}

func (d *CloudsigmaDeployer) createServer(logger *log.Entry, name, pw string, cpu, mem int, disk ResourceType, dataDisk ResourceType, ip IPReferenceType) (ResourceType, error) {
	conf := "static"

	servers := RequestResponseType{
		Objects: []ResourceType{ResourceType{
			Name:        name,
			CPU:         cpu,
			Mem:         mem,
			VNCPassword: pw,
			Drives: []ServerDriveType{
				ServerDriveType{
					BootOrder:  1,
					DevChannel: "0:0",
					Device:     "virtio",
					Drive:      disk},
				ServerDriveType{
					BootOrder:  2,
					DevChannel: "0:1",
					Device:     "virtio",
					Drive:      dataDisk}},
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
			SMP: 2,
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

func (d *CloudsigmaDeployer) CreateServer(resource blueprint.ResourceType, ip IPReferenceType, pfx string, c chan NodeCreationResult) error {
	nodeName := strings.ToLower(pfx + "-" + resource.Name)
	logger := log.WithField("server", nodeName)
	result := NodeCreationResult{
		Info: ditas.NodeInfo{
			Role:     strings.ToLower(resource.Role),
			Name:     nodeName,
			Username: "cloudsigma",
		},
	}
	logger.Info("Creating node", nodeName)

	pw, err := password.Generate(10, 3, 2, false, false)

	if err != nil {
		return d.returnError(logger, fmt.Sprintf("Error generating random password: %s\n", err.Error()), result, err, c)
	}

	cpu, err := strconv.Atoi(resource.CPUs)
	if err != nil {
		return d.returnError(logger, fmt.Sprintf("Error parsing CPU value. It should be an number (of Mhz), found %s: %s\n", resource.CPUs, err.Error()), result, err, c)
	}

	if cpu < 4000 && result.Info.Role == "master" {
		logger.Warnf("The minimum requirements for a Kubernetes master are 2 cores at 2Ghz. %d of CPU might be insufficient and the Kubernetes deploy might fail due to timeouts.", cpu)
	}

	mem, err := strconv.Atoi(resource.RAM)
	if err != nil {
		return d.returnError(logger, fmt.Sprintf("Error parsing RAM value. It should be an number (of Mb), found %s: %s\n", resource.RAM, err.Error()), result, err, c)
	}

	if mem < 2024 && result.Info.Role == "master" {
		logger.Warnf("The minimum requirements for a Kubernetes master are 2GB of RAM. %d MB might be insufficient and the Kubernetes deploy might fail due to timeouts.", mem)
	}

	mem = mem * 1024 * 1024

	disks, err := d.createHostDrives(logger, resource)
	result.Info.DriveUUID = disks.Drive.UUID
	result.Info.DataDriveUUID = disks.Data.UUID
	if err != nil {
		return d.returnError(logger, "Error creating disks", result, err, c)
	}

	logger.Infof("Creating server")

	server, err := d.createServer(logger, nodeName, pw, cpu, mem, disks.Drive, disks.Data, ip)
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

func (d *CloudsigmaDeployer) deletePartialDeployment(nodeInfo NodeCreationResult) error {
	logger := log.NewEntry(log.New())
	if nodeInfo.Info.UUID != "" {
		logger = logger.WithField("server", nodeInfo.Info.UUID)
	}

	if nodeInfo.Info.DriveUUID != "" {
		logger = logger.WithField("disk", nodeInfo.Info.DriveUUID)
	}
	logger.Info("Undoing partial deployment...")
	return d.deleteHost(logger, nodeInfo.Info)
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

func (d *CloudsigmaDeployer) DeployInfrastructure(infrastructure blueprint.InfrastructureType, namePrefix string) (ditas.InfrastructureDeployment, error) {

	numNodes := len(infrastructure.Resources)
	deployment := ditas.InfrastructureDeployment{
		ID:     namePrefix + "_cs",
		Type:   DeploymentType,
		Slaves: make([]ditas.NodeInfo, 0, numNodes-1),
	}

	var logger = log.WithField("deployment", infrastructure.Name)

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

	for i, resource := range infrastructure.Resources {
		go d.CreateServer(resource, ips[i], namePrefix, c)
	}

	var failed = false

	for remaining := numNodes; remaining > 0; remaining-- {
		result := <-c
		if result.Error == nil {
			if strings.ToLower(result.Info.Role) == "master" {
				deployment.Master = result.Info
			} else {
				deployment.Slaves = append(deployment.Slaves, result.Info)
			}
		} else {
			failed = true
		}
	}

	if failed {
		logger.Errorf("Deployment failed")
		return deployment, errors.New("Deployment failed")
	}
	logger.Infof("Nodes successfully created")

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

func (d *CloudsigmaDeployer) deleteHost(log *log.Entry, host ditas.NodeInfo) error {

	logger := log.WithField("host", host.Name)
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
	if host.DriveUUID != "" {
		err = d.deleteDrive(logger, host.DriveUUID)
	}

	if err == nil && host.DataDriveUUID != "" {
		err = d.deleteDrive(logger, host.DataDriveUUID)
	}
	return err

}

func (d *CloudsigmaDeployer) DeleteInfrastructure(infra ditas.InfrastructureDeployment, bpName string) map[string]error {
	logger := log.WithField("blueprint", bpName)

	logger.Info("Deleting infrastructure")

	result := make(map[string]error)

	logger.Info("Deleting slaves")
	for _, slave := range infra.Slaves {
		errSlave := d.deleteHost(logger, slave)
		if errSlave != nil {
			logger.WithError(errSlave).Errorf("Error deleting slave %s", slave.Name)
			result[slave.Name] = errSlave
		}
	}

	if len(result) == 0 {
		logger.Info("Slaves deleted")
	}

	logger.Info("Now deleting master")
	errMaster := d.deleteHost(logger, infra.Master)
	if errMaster != nil {
		logger.WithError(errMaster).Errorf("Error deleting master %s", infra.Master.Name)
		result[infra.Master.Name] = errMaster
	} else {
		logger.Info("Master deleted. Infrastructure clear")
	}

	return result
}
