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
	result.Error = d.deletePartialDeployment(result)
	c <- result
	return err
}

func (d *CloudsigmaDeployer) cloneDisk(logInput *log.Entry, resource blueprint.ResourceType, result *NodeCreationResult, c chan NodeCreationResult) (*ResourceType, error) {
	params := map[string]string{
		"distribution": resource.OS,
		"version":      resource.BaseImage,
	}
	logger := logInput.WithField("disk", resource.OS+" "+resource.BaseImage)
	logger.Infof("Finding disk image of OS %s and version %s\n", resource.OS, resource.BaseImage)
	drive, err := d.client.GetLibDrive(params)

	if err != nil {
		return nil, d.returnError(logger, fmt.Sprintf("Error finding disk image: %s\n", err.Error()), *result, err, c)
	}

	if drive.UUID == "" {
		msg := fmt.Sprintf("Empty UUID for drive when looking for OS %s and version %s", resource.OS, resource.BaseImage)
		return nil, d.returnError(logger, msg, *result, errors.New(msg), c)
	}

	logger.Infof("Image found. Cloning disk %s\n", drive.UUID)

	cloned, err := d.client.CloneDrive(drive.UUID)

	if err != nil {
		return nil, d.returnError(logger, fmt.Sprintf("Error cloning disk: %s\n", err.Error()), *result, err, c)
	}

	if cloned.UUID == "" {
		msg := fmt.Sprintf("Empty UUID found for cloned drive of %s", drive.UUID)
		return nil, d.returnError(logger, msg, *result, errors.New(msg), c)
	}

	result.Info.DriveUUID = cloned.UUID

	logger.Info("Disk cloned. Waiting for it to be ready...")
	timeout := 60 * time.Second
	cloned, timedOut, err := d.waitForStatusChange(cloned.UUID, "cloning_dst", timeout, d.client.GetDriveDetails)

	if err != nil {
		return nil, d.returnError(logger, fmt.Sprintf("Error waiting for disk %s to be ready!: %s", cloned.UUID, err.Error()), *result, err, c)
	}

	if timedOut {
		return nil, d.returnError(logger, fmt.Sprintf("Timeout waiting for disk %s to be ready", cloned.UUID), *result, err, c)
	}

	if cloned.Status != "unmounted" {
		msg := fmt.Sprintf("Drive in unexpected state: %s", cloned.Status)
		return nil, d.returnError(logger, msg, *result, errors.New(msg), c)
	}

	logger.Infof("Disk %s Ready!", cloned.UUID)

	return &cloned, nil
}

func (d *CloudsigmaDeployer) createServer(logger *log.Entry, name, pw string, cpu, mem int, disk ResourceType, result NodeCreationResult, c chan NodeCreationResult) (*ResourceType, error) {
	servers := RequestResponseType{
		Objects: []ResourceType{ResourceType{
			Name:        name,
			CPU:         cpu,
			Mem:         mem,
			VNCPassword: pw,
			Drives: []ServerDriveType{ServerDriveType{
				BootOrder:  1,
				DevChannel: "0:0",
				Device:     "virtio",
				Drive:      disk,
			}},
			NICS: []ServerNICType{ServerNICType{
				IPV4Conf: ServerIPV4ConfType{
					Conf: "dhcp",
				},
				Model: "virtio",
			}},
			Meta: map[string]string{
				"ssh_public_key": d.publicKey,
			},
		}},
	}

	servers, err := d.client.CreateServers(servers)

	if err != nil {
		return nil, d.returnError(logger, fmt.Sprintf("Error creating server: %s", err.Error()), result, err, c)
	}

	if len(servers.Objects) == 0 {
		msg := "The server could not be created but we didn't get an error"
		return nil, d.returnError(logger, msg, result, errors.New(msg), c)
	}

	server := servers.Objects[0]

	if server.UUID == "" {
		msg := "Server created without an UUID"
		return nil, d.returnError(logger, msg, result, errors.New(msg), c)
	}

	logger.Info("Server created!")
	return &server, nil
}

func (d *CloudsigmaDeployer) startServer(logger *log.Entry, uuid string, result NodeCreationResult, c chan NodeCreationResult) (*ResourceType, error) {
	logger.Info("Starting server")
	actionResult, err := d.client.ExecuteServerAction(uuid, ServerStartAction)
	if err != nil {
		return nil, d.returnError(logger, fmt.Sprintf("Error starting server %s: %s", uuid, err.Error()), result, err, c)
	}

	if actionResult.Result != "success" {
		msg := fmt.Sprintf("Unexpected state of start operation for server %s: %s", uuid, actionResult.Result)
		return nil, d.returnError(logger, msg, result, errors.New(msg), c)
	}
	logger.Info("Server booting")

	timeout := 120 * time.Second
	server, timedOut, err := d.waitForStatusChange(uuid, "starting", timeout, d.client.GetServerDetails)

	logger.Infof("Waiting for server to start")

	if err != nil {
		return nil, d.returnError(logger, fmt.Sprintf("Error waiting for server %s to be ready!: %s", server.UUID, err.Error()), result, err, c)
	}

	if timedOut {
		return nil, d.returnError(logger, fmt.Sprintf("Timeout waiting for server %s to be ready", server.UUID), result, err, c)
	}

	logger.Info("Server started!!")

	return &server, nil
}

func (d *CloudsigmaDeployer) CreateServer(resource blueprint.ResourceType, pfx string, c chan NodeCreationResult) error {
	nodeName := pfx + "-" + resource.Name
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

	mem, err := strconv.Atoi(resource.RAM)
	if err != nil {
		return d.returnError(logger, fmt.Sprintf("Error parsing RAM value. It should be an number (of Mb), found %s: %s\n", resource.RAM, err.Error()), result, err, c)
	}

	mem = mem * 1024 * 1024

	cloned, err := d.cloneDisk(logger, resource, &result, c)
	if err != nil {
		return err
	}

	logger.Infof("Creating server")

	server, err := d.createServer(logger, nodeName, pw, cpu, mem, *cloned, result, c)
	if err != nil {
		return err
	}

	result.Info.UUID = server.UUID

	server, err = d.startServer(logger, server.UUID, result, c)
	if err != nil {
		return err
	}

	if len(server.Runtime.NICs) == 0 {
		msg := "Can't find network information for server"
		return d.returnError(logger, msg, result, errors.New(msg), c)
	}

	ip := server.Runtime.NICs[0].IPV4Info.UUID

	if ip == "" {
		msg := "Can't find IP address for server"
		return d.returnError(logger, msg, result, errors.New(msg), c)
	}

	result.Info.IP = ip

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

func (d *CloudsigmaDeployer) DeployInfrastructure(infrastructure blueprint.InfrastructureType, namePrefix string) (ditas.InfrastructureDeployment, error) {

	numNodes := len(infrastructure.Resources)
	deployment := ditas.InfrastructureDeployment{
		ID:     namePrefix + "_cs",
		Type:   DeploymentType,
		Slaves: make([]ditas.NodeInfo, 0, numNodes-1),
	}

	var logger = log.WithField("deployment", infrastructure.Name)

	c := make(chan NodeCreationResult, numNodes)

	for _, resource := range infrastructure.Resources {
		go d.CreateServer(resource, namePrefix, c)
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

func (d *CloudsigmaDeployer) deleteHost(log *log.Entry, host ditas.NodeInfo) error {

	logger := log.WithField("host", host.Name)
	if host.UUID != "" {
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
		logger.Info("Server stopped. Deleting")

		err = d.client.DeleteServerWithDrives(host.UUID)

		if err != nil {
			logger.WithError(err).Error("Error deleting server with drives")
			return err
		}
		logger.Info("Host successfully deleted")
		return nil
	}

	if host.DriveUUID != "" {
		logger.Info("Deleting drive from host")
		err := d.client.DeleteDrive(host.DriveUUID)
		if err != nil {
			logger.WithError(err).Error("Error deleting drive")
			return err
		}
		logger.Info("Drive deleted")
		return nil
	}

	return nil

}

func (d *CloudsigmaDeployer) DeleteInfrastructure(infra ditas.InfrastructureDeployment, bpName string) map[string]error {
	logger := log.WithField("blueprint", bpName)

	logger.Info("Deleting infrastructure")

	result := make(map[string]error)

	logger.Info("Deleting slaves")
	for _, slave := range infra.Slaves {
		errSlave := d.deleteHost(logger, slave)
		if errSlave != nil {
			logger.WithError(errSlave).Error("Error deleting slave %s", slave.Name)
			result[slave.Name] = errSlave
		}
	}

	if len(result) == 0 {
		logger.Info("Slaves deleted")
	}

	logger.Info("Now deleting master")
	errMaster := d.deleteHost(logger, infra.Master)
	if errMaster != nil {
		logger.WithError(errMaster).Error("Error deleting master %s", infra.Master.Name)
		result[infra.Master.Name] = errMaster
	} else {
		logger.Info("Master deleted. Infrastructure clear")
	}

	return result
}
