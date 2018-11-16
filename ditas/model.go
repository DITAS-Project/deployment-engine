package ditas

type VDCInformation struct {
	ID           string `bson:"_id"`
	DeploymentID string
	InfraVDCs    map[string]InfraServicesInformation
}

type InfraServicesInformation struct {
	LastPort    int
	VdcNumber   int
	Initialized bool
	VdcPorts    map[string]int
}
