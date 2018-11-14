package deployers

import (
	"deployment-engine/src/cloudsigma"
	"deployment-engine/src/ditas"
	"errors"
	"net"
	"strconv"
	"strings"

	blueprint "github.com/DITAS-Project/blueprint-go"
	"github.com/apcera/libretto/ssh"
	"github.com/sethvargo/go-password/password"
	log "github.com/sirupsen/logrus"
)

type CloudSigmaVirtualMachine struct {
	Name         string
	Ips          []net.IP
	State        string
	ResourceInfo blueprint.ResourceType
}

type NodeCreationResult struct {
	Info  ditas.NodeInfo
	Error error
}

type HostDisks struct {
	Drive cloudsigma.ResourceType
	Data  cloudsigma.ResourceType
}

func (vm *CloudSigmaVirtualMachine) GetName() string {
	return vm.Name
}

func (vm *CloudSigmaVirtualMachine) Provision() error {

}

func (vm *CloudSigmaVirtualMachine) GetIPs() ([]net.IP, error) {
	return vm.Ips, nil
}

func (vm *CloudSigmaVirtualMachine) Destroy() error {

}

func (vm *CloudSigmaVirtualMachine) GetState() (string, error) {
	return vm.State, nil
}

func (vm *CloudSigmaVirtualMachine) Suspend() error {

}

func (vm *CloudSigmaVirtualMachine) Resume() error {

}

func (vm *CloudSigmaVirtualMachine) Halt() error {

}

func (vm *CloudSigmaVirtualMachine) Start() error {

}

func (vm *CloudSigmaVirtualMachine) GetSSH(ssh.Options) (ssh.Client, error) {

}

func (d *CloudSigmaVirtualMachine) createHostDrives(logInput *log.Entry, resource blueprint.ResourceType) (HostDisks, error) {
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

func (d *CloudSigmaVirtualMachine) CreateServer(ip IPReferenceType, pfx string) (NodeCreationResult, error) {
	resource := d.ResourceInfo
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
		logger.WithError(err).Error("Error generating random password")
		return result, err
	}

	cpu, err := strconv.Atoi(resource.CPUs)
	if err != nil {
		logger.WithError(err).Errorf("Error parsing CPU value. It should be an number (of Mhz), found %s", resource.CPUs)
		return result, err
	}

	if cpu < 4000 && result.Info.Role == "master" {
		logger.Warnf("The minimum requirements for a Kubernetes master are 2 cores at 2Ghz. %d of CPU might be insufficient and the Kubernetes deploy might fail due to timeouts.", cpu)
	}

	mem, err := strconv.Atoi(resource.RAM)
	if err != nil {
		logger.WithError(err).Errorf("Error parsing RAM value. It should be an number (of Mb), found %s", resource.RAM)
		return result, err
	}

	if mem < 2024 && result.Info.Role == "master" {
		logger.Warnf("The minimum requirements for a Kubernetes master are 2GB of RAM. %d MB might be insufficient and the Kubernetes deploy might fail due to timeouts.", mem)
	}

	mem = mem * 1024 * 1024

	disks, err := d.createHostDrives(logger, resource)
	result.Info.DriveUUID = disks.Drive.UUID
	result.Info.DataDriveUUID = disks.Data.UUID
	if err != nil {
		logger.WithError(err).Error("Error creating disks")
		return result, err
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
