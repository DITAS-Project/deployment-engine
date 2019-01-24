/*
Copyright 2017 Atos

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package blueprint

import (
	"github.com/go-openapi/spec"
)

// Drive contains the information of a data drive
// swagger:model
type Drive struct {
	// Name of the image to use. Most of the times, it will be available as /dev/disk/by-id/${name} value in the VM
	// required: true
	Name string `json:"name"`

	// Type of the drive. It can be "SSD" or "HDD"
	// required: true
	// pattern: SSD|HDD
	// example: SSD
	Type string `json:"type"`

	// Size of the disk in MB
	// required: true
	Size int64 `json:"size"`
}

// ResourceType represents a resource such as a virtual machine
// swagger:model
type ResourceType struct {
	// Suffix for the hostname. The real hostname will be formed of the infrastructure name + resource name
	Name string `json:"name"`

	// Type of the VM to create
	// example: n1-small
	Type string `json:"type"`

	// CPU speed in MHz. Ignored if type is provided
	CPU int `json:"cpu"`

	// Number of cores. Ignored if type is provided
	Cores int `json:"cores"`

	// RAM quantity in MB. Ignored if type is provided
	RAM int64 `json:"ram"`

	// Boot disk size in MB
	Disk int64 `json:"disk"`

	// Role that this VM plays. In case of a Kubernetes deployment at least one "master" is needed.
	// required: true
	// pattern: master|slave
	// example: master
	Role string `json:"role"`

	// Boot image ID to use
	// required: true
	ImageId string `json:"image_id"`

	// IP to assign this VM. In case it's not specified, the first available one will be used.
	IP string `json:"ip,omitempty"`

	// List of data drives to attach to this VM
	Drives []Drive `json:"drives"`
}

// CloudProviderInfo contains information about a cloud or edge provider
// swagger:model
type CloudProviderInfo struct {
	// Endpoint to use for this infrastructure
	APIEndpoint string `json:"api_endpoint"`

	// Type of the infrastructure. i.e AWS, Cloudsigma, GCP or Edge
	// example: AWS
	APIType string `json:"api_type"`

	// Keypair to use to log in to the infrastructure manager
	KeypairID string `json:"keypair_id"`
}

// InfrastructureType represents a cloud or edge site that's able to create resources such as virtual machines or volumes
// swagger:model
type InfrastructureType struct {
	// Unique name for the infrastructure
	// required: true
	// unique: true
	Name string `json:"name"`

	// Optional description for the infrastructure
	Description string `json:"description"`

	// Type of the infrastructure
	// required: true
	// pattern: Cloud|Edge
	// example: Cloud
	Type string `json:"type"`

	// Provider information
	// required: true
	Provider CloudProviderInfo `json:"provider"`

	// List of resources to deploy
	// required: true
	Resources []ResourceType `json:"resources"`
}

// CookbookAppendix is the definition of the Cookbook Appendix section in the blueprint
// swagger:model
type CookbookAppendix struct {
	// Unique name of the deployment
	// required: true
	// unique: true
	Name string `json:"name"`

	// An optional description for the deployment
	// required: false
	Description string `json:"description"`

	// A list of infrastructures that should be initialized to deploy VDCs of this blueprint
	// required: true
	Infrastructure []InfrastructureType `json:"infrastructure"`
}

// LeafType is a leaf in a tree data structure
// swagger:model
type LeafType struct {
	// Unique identifier for the leaf
	// required: true
	// unique: true
	Id *string `json:"id"`

	// An optional description for the leaf
	// required: false
	Description string `json:"description"`

	// The weight in the resolution of the constraint
	// requiered: true
	Weight int `json:"weight"`

	// The list of attributes defined in the data management section to match. All of them must comply.
	// requiered: true
	Attributes []string `json:"attributes"`
}

// TreeStructureType is a tree structure with a root and subtrees or leaves
// swagger:model
type TreeStructureType struct {

	// The operation to apply to the subtree or leaves
	// required: true
	// pattern: AND|OR
	// example: AND
	Type *string `json:"type"`

	// The subtrees pending from this node
	// required: false
	Children []TreeStructureType `json:"children"`

	// The leaves pending from this node
	// required: false
	Leaves []LeafType `json:"leaves"`
}

// GoalTreeType defines a goal tree
// swagger:model
type GoalTreeType struct {

	// Goal tree for data utility properties
	// required: false
	DataUtility TreeStructureType `json:"dataUtility`

	// Goal tree for security properties
	// required: false
	Security TreeStructureType `json:"security`

	// Goal tree for privacy properties
	// required: false
	Privacy TreeStructureType `json:"privacy`
}

// AbstractPropertiesMethodType defines a goal tree for a method
// swagger:model
type AbstractPropertiesMethodType struct {

	// The method identifier this goals apply to
	// required: true
	MethodId *string `json:"method_id"`

	// The goal tree for this method
	// required: true
	GoalTrees GoalTreeType `json:"goalTrees"`
}

// ConstraintType is the definition of a constraint threshold.
// Either maximum, minimum or value is required.
// swagger:model
type MetricPropertyType struct {
	// The units in which this property is measured
	// required: true
	// example: MB/s
	Unit string `json:"unit"`

	// The minimum value for the threshold
	// required: false
	Minimum *float64 `json:"minimum"`

	// The maximum value for the threshold
	// required: false
	Maximum *float64 `json:"maximum"`

	// The value this property must maintain
	// required: false
	Value *interface{} `json:"value"`
}

// IsMinimumConstraint test if the MetricPropertyType has a minimum constraint
func (m *MetricPropertyType) IsMinimumConstraint() bool {
	return m.Minimum != nil
}

// IsMaximumConstraint test if the MetricPropertyType has a maximum constraint
func (m *MetricPropertyType) IsMaximumConstraint() bool {
	return m.Maximum != nil
}

// IsEqualityConstraint test if the MetricPropertyType has only a value and no min or max constraints
func (m *MetricPropertyType) IsEqualityConstraint() bool {
	return m.Value != nil && m.Maximum == nil && m.Minimum == nil
}

// ConstraintType is the definition of a QoS constraint
// swagger:model
type ConstraintType struct {

	// A unique identifier for the constraint
	// required: true
	ID *string `json:"id"`

	// An optional description for the constraint
	// required: false
	Description string `json:"description"`

	// The type of the constraint
	// required: true
	// example: Accuracy
	Type string `json:"type"`

	// The set of properties thresholds associated to this constraints
	// required: true
	// example: "accuracy": { "minimum": 0.9, "unit": "none" }
	Properties map[string]MetricPropertyType `json:"properties"`
}

// DataManagementAttributesType contains the data managements values associated to a method
// swagger:model
type DataManagementAttributesType struct {

	// The constraints associated to data utility
	// required: false
	DataUtility []ConstraintType `json:"dataUtility"`

	// The constraints associated to security
	// required: false
	Security []ConstraintType `json:"security"`

	// The constraints associated to privacy
	// requiered: false
	Privacy []ConstraintType `json:"privacy"`
}

// DataManagementMethodType contains the data management attributes associated to a method
// swagger:model
type DataManagementMethodType struct {

	// The unique method id this attributes apply to
	// required: true
	MethodId *string `json:"method_id"`

	// The attributes to apply to this method
	// required: true
	Attributes DataManagementAttributesType `json:"attributes"`
}

// MethodTagType is a structure to define tags per methos
// swagger:model
type MethodTagType struct {

	// The method identifier
	// required: true
	ID string `json:"method_id"`

	// The list of tags to apply to the method
	// required: false
	Tags []string `json:"tags"`
}

// OverviewType are general descriptive properties of the blueprint
// swagger:model
type OverviewType struct {

	// A unique name for the blueprint. It will be identified by this property.
	// required: true
	Name *string `json:"Name"`

	// A list of tags to apply to this blueprint
	// required: false
	Tags []MethodTagType `json:"tags"`
}

// DataSourceType is a datasource definition
// swagger:model
type DataSourceType struct {

	// The unique identifier of the datasource
	// required: true
	ID *string `json:"id"`

	// The type of the datasource
	// required: true
	Type *string `json:"type"`

	// A map of parameters relevant for the datasource
	// required: false
	Parameters map[string]interface{} `json:"parameters"`
}

// InternalStructureType is the serialization of a DITAS concrete blueprint
// swagger:model
type InternalStructureType struct {

	// The overview section
	// required: true
	Overview OverviewType `json:"Overview"`

	// The datasources description
	// required: true
	DataSources []DataSourceType `json:"Data_Sources"`
}

// BlueprintType is the serialization of a DITAS concrete blueprint
// swagger:model
type BlueprintType struct {
	// The internal structure section
	// required: true
	InternalStructure InternalStructureType `json:"INTERNAL_STRUCTURE"`

	// The data management section
	// required: true
	DataManagement []DataManagementMethodType `json:"DATA_MANAGEMENT"`

	// The abstract properties section
	// required: true
	AbstractProperties []AbstractPropertiesMethodType `json:"ABSTRACT_PROPERTIES"`

	// The blueprint API description section
	// required: true
	API spec.Swagger `json:"EXPOSED_API"`

	// The cookbook appendix section containing the available resources
	// required: true
	CookbookAppendix CookbookAppendix `json:"COOKBOOK_APPENDIX"`
}
