package ditas

import (
	blueprint "github.com/DITAS-Project/blueprint-go"
)

type NodeInfo struct {
	Name      string `json:"name"`
	Role      string `json:"role"`
	IP        string `json:"ip"`
	Username  string `json:"username"`
	UUID      string `json:"uuid"`
	DriveUUID string `json:"drive_uuid" bson:"drive_uuid"`
}

type InfrastructureDeployment struct {
	ID      string                             `json:"id"`
	Type    string                             `json:"type"`
	Slaves  []NodeInfo                         `json:"slaves"`
	Master  NodeInfo                           `json:"master"`
	NumVDCs int                                `json:"num_vdcs" bson:"num_vdcs"`
	Status  string                             `json:"status"`
	VDCs    map[string]blueprint.BlueprintType `json:"vdcs"`
}

type Deployment struct {
	ID              string                     `json:"id" bson:"_id"`
	Infrastructures []InfrastructureDeployment `json:"infrastructures"`
	Status          string                     `json:"status"`
}

type Deployer interface {
	DeployInfrastructure(infrastructure blueprint.InfrastructureType, namePrefix string) (InfrastructureDeployment, error)
}
