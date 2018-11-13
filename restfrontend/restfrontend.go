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
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

type App struct {
	Router                *mux.Router
	DeploymentController  *infrastructure.Deployer
	ProvisionerController *provision.ProvisionerController
}

func New(repository persistence.DeploymentRepository, provisioner model.Provisioner) *App {
	result := App{
		Router: mux.NewRouter(),
		DeploymentController: &infrastructure.Deployer{
			Repository: repository,
		},
		ProvisionerController: &provision.ProvisionerController{
			Repository:  repository,
			Provisioner: provisioner,
		},
	}
	result.initializeRoutes()
	return &result
}

func (a App) Run(addr string) error {
	return http.ListenAndServe(addr, a.Router)
}

func (a *App) initializeRoutes() {
	a.Router.HandleFunc("/deployment", a.createDep).Methods("POST")
	a.Router.HandleFunc("/deployment/{depId}/{infraId}", a.deleteInfra).Methods("DELETE")
	a.Router.HandleFunc("/deployment/{depId}/{infraId}/{product}", a.deployProduct).Methods("PUT")
	/*a.Router.HandleFunc("/deployment", a.getAllDeps).Methods("GET")
	a.Router.HandleFunc("/deployment/{id}", a.getDep).Methods("GET")
	a.Router.HandleFunc("/deployment/{id}", a.deleteDep).Methods("DELETE")*/

}

func (a *App) getQueryParam(key string, r *http.Request) (string, bool) {
	values := mux.Vars(r)
	if values != nil {
		ok, val := values[key]
		return ok, val
	}

	return "", false
}

func (a *App) createDep(w http.ResponseWriter, r *http.Request) {
	var deployment model.Deployment
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&deployment); err != nil {
		log.WithError(err).Error("Error deserializing deployment")
		respondWithError(w, http.StatusBadRequest, fmt.Sprintf("Invalid payload: %s", err.Error()))
		return
	}
	defer r.Body.Close()

	result, err := a.DeploymentController.CreateDeployment(deployment)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondWithJSON(w, http.StatusCreated, result)

	//respondWithJSON(w, http.StatusOK, map[string]string{"result": "success"})
}

func (a *App) deleteInfra(w http.ResponseWriter, r *http.Request) {
	depId, ok := a.getQueryParam("depId", r)
	if !ok {
		respondWithError(w, http.StatusBadRequest, "Can't find deployment ID parameter")
	}

	infraId, ok := a.getQueryParam("infraId", r)
	if !ok {
		respondWithError(w, http.StatusBadRequest, "Can't find infrastructure ID parameter")
	}

	dep, err := a.DeploymentController.DeleteInfrastructure(depId, infraId)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Error deleting infrastructure: %s", err.Error()))
	}

	respondWithJSON(w, http.StatusOK, dep)
}

func (a *App) deployProduct(w http.ResponseWriter, r *http.Request) {
	depId, ok := a.getQueryParam("depId", r)
	if !ok {
		respondWithError(w, http.StatusBadRequest, "Can't find deployment ID parameter")
	}

	infraId, ok := a.getQueryParam("infraId", r)
	if !ok {
		respondWithError(w, http.StatusBadRequest, "Can't find infrastructure ID parameter")
	}

	product, ok := a.getQueryParam("product", r)
	if !ok {
		respondWithError(w, http.StatusBadRequest, "Can't find product parameter")
	}

	deployment, err := a.ProvisionerController.Provision(depId, infraId, product)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Error deploying product: %s", err.Error()))
	}

	respondWithJSON(w, http.StatusOK, deployment)
}

/*func (a *App) deployKubernetes(w http.ResponseWriter, r *http.Request) {

}

func (a *App) getAllDeps(w http.ResponseWriter, r *http.Request) {
	deps, err := a.Controller.GetAllDeps()
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondWithJSON(w, http.StatusOK, deps)
}

func (a *App) getDep(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	dep, err := a.Controller.GetDep(id)
	if err != nil {
		switch err {
		case mgo.ErrNotFound:
			respondWithError(w, http.StatusNotFound, err.Error())
		default:
			respondWithError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	respondWithJSON(w, http.StatusOK, dep)
}

func (a *App) deleteDep(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	vars := mux.Vars(r)
	id, ok := vars["id"]

	if !ok {
		respondWithError(w, http.StatusBadRequest, "Deployment id is mandatory")
		return
	}

	vdcID := getQueryParam("vdc", values)
	deleteDeployment, err := strconv.ParseBool(getQueryParam("deleteDeployment", values))
	if err != nil {
		fmt.Printf("deleteDeployment parameter not found or invalid. Assuming false")
		deleteDeployment = false
	}

	if err := a.Controller.DeleteVDC(id, vdcID, deleteDeployment); err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, map[string]string{"result": "success"})
}*/

func respondWithError(w http.ResponseWriter, code int, message string) {
	respondWithJSON(w, code, map[string]string{"error": message})
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, _ := json.Marshal(payload)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(response)
}
