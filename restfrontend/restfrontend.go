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
package restfrontend

import (
	"deployment-engine/infrastructure"
	"deployment-engine/model"
	"deployment-engine/persistence"
	"deployment-engine/provision"
	"deployment-engine/provision/ansible"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/julienschmidt/httprouter"
	log "github.com/sirupsen/logrus"
)

type App struct {
	Router                *httprouter.Router
	DeploymentController  *infrastructure.Deployer
	ProvisionerController *provision.ProvisionerController
	Vault                 persistence.Vault
}

func New(repository persistence.DeploymentRepository, vault persistence.Vault, publicKeyPath string) (*App, error) {
	ansibleProvisioner, err := ansible.New()
	if err != nil {
		return nil, err
	}

	result := App{
		Router: httprouter.New(),
		DeploymentController: &infrastructure.Deployer{
			Repository:    repository,
			Vault:         vault,
			PublicKeyPath: publicKeyPath,
		},
		ProvisionerController: provision.NewProvisionerController(ansibleProvisioner, repository),
		Vault:                 vault,
	}
	result.InitializeRoutes()
	return &result, nil
}

func (a App) Run(addr string) error {
	return http.ListenAndServe(addr, a.Router)
}

func (a *App) InitializeRoutes() {
	a.Router.POST("/infra", a.CreateDep)
	a.Router.DELETE("/infra", a.DeleteDeployment)
	a.Router.DELETE("/infra/:infraId", a.DeleteInfra)
	a.Router.POST("/infra/:infraId/:framework/:product", a.DeployProduct)
	a.Router.POST("/secrets", a.CreateSecret)
}

func (a *App) ReadBody(r *http.Request, result interface{}) error {
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(result); err != nil {
		log.WithError(err).Error("Error deserializing deployment")
		return fmt.Errorf("Invalid payload: %s", err.Error())
	}
	return nil
}

// CreateDep creates a new deployment
func (a *App) CreateDep(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	// swagger:operation POST /infra deployment createDeployment
	//
	// Creates a multi-cluster deployment with the by instantiating the infrastructures passed as parameter.
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
	//   description: The deployment description
	//   required: true
	//   schema:
	//     $ref: "#/definitions/Deployment"
	//
	// responses:
	//   201:
	//     description: Deployment successfully created
	//     schema:
	//       $ref: "#/definitions/DeploymentInfo"
	//   400:
	//     description: Bad request
	//   500:
	//     description: Internal error
	defer r.Body.Close()

	var deployment []model.InfrastructureType
	if err := a.ReadBody(r, &deployment); err != nil {
		RespondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := a.DeploymentController.CreateDeployment(deployment)

	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	RespondWithJSON(w, http.StatusCreated, result)
	return
}

// DeleteDeployment deletes an existing deployment
func (a *App) DeleteDeployment(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	// swagger:operation DELETE /infra deployment deleteDeployment
	//
	// Deletes a list of infrastructures
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
	// - name: depId
	//   in: query
	//   type: string
	//   description: A comma-separated list of infrastructure identifiers to delete
	//
	// responses:
	//   204:
	//     description: Deployment successfully deleted
	//   400:
	//     description: Bad request
	//   500:
	//     description: Internal error
	depIds := r.URL.Query().Get("depId")
	if depIds == "" {
		RespondWithError(w, http.StatusBadRequest, "Can't find deployment ID parameter")
		return
	}

	err := a.DeploymentController.DeleteDeployment(strings.Split(depIds, ","))
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Error deleting deployment: %s", err.Error()))
		return
	}

	Respond(w, http.StatusNoContent, []byte{}, "plain/text")
	return
}

// DeleteInfra deletes an existing infrastructure
func (a *App) DeleteInfra(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	// swagger:operation DELETE /infra/{infraId} deployment deleteInfrastructure
	//
	// Deletes an infrastructure in a deployment
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
	// - name: infraId
	//   in: path
	//   required: true
	//   type: string
	//   description: The infrastructure identifier to delete
	//
	// responses:
	//   200:
	//     description: Infrastructure successfully deleted. Returns the updated deployment
	//     schema:
	//       $ref: "#/definitions/InfrastructureDeploymentInfo"
	//   400:
	//     description: Bad request
	//   500:
	//     description: Internal error
	infraId := ps.ByName("infraId")
	if infraId == "" {
		RespondWithError(w, http.StatusBadRequest, "Can't find infrastructure ID parameter")
		return
	}

	dep, err := a.DeploymentController.DeleteInfrastructure(infraId)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Error deleting infrastructure: %s", err.Error()))
		return
	}

	RespondWithJSON(w, http.StatusOK, dep)
	return
}

// DeployProduct deploys a new product in an infrastructure
func (a *App) DeployProduct(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	// swagger:operation POST "/infra/{infrastructureId}/{framework}/{product}" deployment createProduct
	//
	// Creates a Deployment with the by instantiating the infrastructures passed as parameter.
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
	// - name: deploymentId
	//   in: path
	//   description: The deployment in which deploy the product
	// - name: infraId
	//   in: path
	//   description: The infrastructure inside the deployment in which to deploy the product
	// - name: framework
	//   in: path
	//   description: The framework to deploy the product to. It can be either "baremetal" or "kubernetes"
	// - name: product
	//   in: path
	//   description: The software product to deploy
	//
	// responses:
	//   200:
	//     description: The product has been successfully deployed and the updated deployment is returned.
	//     schema:
	//       $ref: "#/definitions/DeploymentInfo"
	//   400:
	//     description: Bad request
	//   500:
	//     description: Internal error
	infraId := ps.ByName("infraId")
	if infraId == "" {
		RespondWithError(w, http.StatusBadRequest, "Can't find infrastructure ID parameter")
		return
	}

	product := ps.ByName("product")
	if product == "" {
		RespondWithError(w, http.StatusBadRequest, "Can't find product parameter")
		return
	}

	framework := ps.ByName("framework")
	if framework == "" {
		RespondWithError(w, http.StatusBadRequest, "Can't find framework parameter")
		return
	}

	params := r.URL.Query()

	deployment, err := a.ProvisionerController.Provision(infraId, product, GetParameters(params), framework)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Error deploying product: %s", err.Error()))
		return
	}

	RespondWithJSON(w, http.StatusOK, deployment)
	return
}

// CreateSecret creates a secret in the configured vault
func (a *App) CreateSecret(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	// swagger:operation POST "/secrets" deployment createProduct
	//
	// Stores a new secret in the configured vault
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
	// - name: secret
	//   in: body
	//   description: The secret description
	//   required: true
	//   schema:
	//     $ref: "#/definitions/Secret"
	//
	// responses:
	//   201:
	//     description: The secret has been saved. Returns the secret Identifier
	//     schema:
	//       type: string
	//   400:
	//     description: Bad request
	//   500:
	//     description: Internal error
	defer r.Body.Close()

	var secret model.Secret
	if err := a.ReadBody(r, &secret); err != nil {
		RespondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	secretID, err := a.Vault.AddSecret(secret)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	Respond(w, http.StatusCreated, []byte(secretID), "plain/text")
	return

}

func GetParameters(args map[string][]string) model.Parameters {
	result := make(model.Parameters)
	for k, v := range args {
		if v != nil {
			if len(v) == 1 {
				result[k] = v[0]
			}

			if len(v) > 1 {
				result[k] = v
			}
		}
	}
	return result
}

func RespondWithError(w http.ResponseWriter, code int, message string) {
	RespondWithJSON(w, code, map[string]string{"error": message})
}

func Respond(w http.ResponseWriter, code int, payload []byte, contentType string) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(code)
	w.Write(payload)
}

func RespondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, _ := json.Marshal(payload)
	Respond(w, code, response, "application/json")
}
