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
	"deployment-engine/infrastructure"
	"deployment-engine/persistence/mongorepo"
	"deployment-engine/provision"
	"deployment-engine/provision/ansible"
	"deployment-engine/restfrontend"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"

	"fmt"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

type DitasFrontend struct {
	DefaultFrontend       *restfrontend.App
	Router                *mux.Router
	DeploymentController  *infrastructure.Deployer
	ProvisionerController *provision.ProvisionerController
	VDCManagerInstance    *VDCManager
}

func NewDitasFrontend() (*DitasFrontend, error) {
	repository, err := mongorepo.CreateRepositoryNative()
	if err != nil {
		return nil, err
	}

	provisioner, err := ansible.New()
	if err != nil {
		return nil, err
	}

	deployer := &infrastructure.Deployer{
		Repository: repository,
	}

	controller := &provision.ProvisionerController{
		Repository:  repository,
		Provisioner: provisioner,
	}

	vdcManager, err := NewVDCManager(provisioner, deployer, controller)
	if err != nil {
		return nil, err
	}

	result := DitasFrontend{
		Router:                mux.NewRouter(),
		DeploymentController:  deployer,
		ProvisionerController: controller,
		DefaultFrontend: &restfrontend.App{
			DeploymentController:  deployer,
			ProvisionerController: controller,
		},
		VDCManagerInstance: vdcManager,
	}

	result.initializeRoutes()
	return &result, nil
}

func (a DitasFrontend) Run(addr string) error {
	return http.ListenAndServe(addr, a.Router)
}

func (a *DitasFrontend) initializeRoutes() {
	a.Router.HandleFunc("/deployment", a.createDep).Methods("POST")
	//a.Router.HandleFunc("/deployment/{depId}/{infraId}", a.DefaultFrontend.deleteInfra).Methods("DELETE")
}

func (a *DitasFrontend) ValidateRequest(request CreateDeploymentRequest) error {

	if request.Blueprint.InternalStructure.Overview.Name == nil {
		return errors.New("Invalid blueprint. Name is mandatory")
	}

	if request.Resources == nil || len(request.Resources) == 0 {
		return errors.New("List of resources to deploy is mandatory")
	}

	for _, infra := range request.Resources {
		provider := infra.Provider
		if provider.APIType != "cloudsigma" {
			return fmt.Errorf("Invalid provider type %s found in infrastructure %s. Only cloudsigma is supported", provider.APIType, infra.Name)
		}

		_, err := url.ParseRequestURI(provider.APIEndpoint)
		if err != nil {
			return fmt.Errorf("The provider endpoint for infrastructure %s is not a valid URL", infra.Name)
		}

		if (provider.Credentials == nil || len(provider.Credentials) == 0) && (provider.SecretID == "") {
			return fmt.Errorf("Credentials or secret identifier are required for provider of infastructure %s", infra.Name)
		}

		if infra.Resources == nil || len(infra.Resources) == 0 {
			return fmt.Errorf("No resources provided for infrastructure %s", infra.Name)
		}

		resNames := make([]string, 0, len(infra.Resources))
		masterFound := false
		storageSpace := int64(0)
		for _, res := range infra.Resources {

			for j := 0; j < len(resNames); j++ {
				if resNames[j] == res.Name {
					return fmt.Errorf("Name of resource %s is not unique in infrastructure %s", res.Name, infra.Name)
				}
			}

			resNames = append(resNames, res.Name)

			minCPU := 2000
			minRAM := int64(2048)
			minDisk := int64(20480)
			if res.Role == "master" {
				masterFound = true
				minCPU = minCPU * 2
				minDisk = minDisk * 2
				minRAM = minRAM * 2
			}

			if res.CPU < minCPU {
				return fmt.Errorf("A minimum of %d CPU is needed for resource %s in infrastructure %s", minCPU, res.Name, infra.Name)
			}

			if res.RAM < minRAM {
				return fmt.Errorf("A minimum of %d RAM is needed for resource %s in infrastructure %s", minRAM, res.Name, infra.Name)
			}

			if res.Disk < minDisk {
				return fmt.Errorf("A minimum of %d size for the boot disk is needed for resource %s in infrastructure %s", minDisk, res.Name, infra.Name)
			}

			if res.ImageId == "" {
				return fmt.Errorf("Empty boot disk found for resource %s in infrastructure %s", res.Name, infra.Name)
			}

			if res.Drives != nil {
				driveNames := make([]string, 0, len(res.Drives))
				for _, drive := range res.Drives {
					for j := 0; j < len(driveNames); j++ {
						if driveNames[j] == drive.Name {
							return fmt.Errorf("Name of drive %s is not unique in resource %s of infrastructure %s", drive.Name, res.Name, infra.Name)
						}
					}

					driveNames = append(driveNames, drive.Name)

					minDisk := int64(5120)
					if drive.Size < minDisk {
						return fmt.Errorf("Size of drive %s in resource %s of infrastructure %s is smaller than the minimum drive size %d", drive.Name, res.Name, infra.Name, minDisk)
					}

					storageSpace += drive.Size
				}
			}
		}

		if !masterFound {
			return fmt.Errorf("Can't find a node with role 'master' in infrastructure %s", infra.Name)
		}

		if storageSpace == 0 {
			return fmt.Errorf("Resources in infrastructure %s don't have space for persistence. Please, include at least one data drive in some resources with at least 5GB of space", infra.Name)
		}

	}

	return nil
}

// swagger:operation POST /deployment deployment createDeployment
//
// Creates a DITAS deployment with the infrastructures passed as parameter.
//
// Creates a Kubernetes installation on each infrastructure and then deploys a VDC on the first one
// based on the blueprint passed as parameter.
//
// ---
// consumes:
// - application/json
//
// produces:
// - application/json
// - text/plain
//
// parameters:
// - name: request
//   in: body
//   description: The request object is composed of an abstract blueprint and a list of resources to use to deploy VDCs
//   required: true
//   schema:
//     $ref: "#/definitions/CreateDeploymentRequest"
//
// responses:
//   200:
//     description: OK
//   400:
//     description: Bad request
//   500:
//     description: Internal error
func (a *DitasFrontend) createDep(w http.ResponseWriter, r *http.Request) {
	var request CreateDeploymentRequest
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&request); err != nil {
		log.WithError(err).Error("Error deserializing deployment")
		restfrontend.RespondWithError(w, http.StatusBadRequest, fmt.Sprintf("Invalid payload: %s", err.Error()))
		return
	}
	defer r.Body.Close()

	if err := a.ValidateRequest(request); err != nil {
		restfrontend.RespondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	err := a.VDCManagerInstance.DeployBlueprint(request)

	if err != nil {
		log.WithError(err).Error("Error deploying blueprint")
		restfrontend.RespondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Error deploying blueprint: %s", err.Error()))
		return
	}

}
