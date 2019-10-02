package ansible

import (
	"deployment-engine/model"

	"github.com/sirupsen/logrus"
)

type HostsProvisioner struct {
	parent *Provisioner
}

func NewHostsProvisioner(parent *Provisioner) HostsProvisioner {
	return HostsProvisioner{
		parent: parent,
	}
}
func (p HostsProvisioner) BuildInventory(infra *model.InfrastructureDeploymentInfo, args model.Parameters) (Inventory, error) {
	return DefaultAllInventory(*infra), nil
}

func (p HostsProvisioner) DeployProduct(inventoryPath string, infra *model.InfrastructureDeploymentInfo, args model.Parameters) (model.Parameters, error) {

	logger := logrus.WithFields(logrus.Fields{
		"infrastructure": infra.ID,
	})

	return nil, ExecutePlaybook(logger, p.parent.ScriptsFolder+"/common/add_hostname.yml", inventoryPath, nil)
}
