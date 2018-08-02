// app.go

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	//"os"
	"strconv"

	blueprint "github.com/DITAS-Project/blueprint-go"
	_ "github.com/go-sql-driver/mysql"
	"github.com/gorilla/mux"
	mongo "github.com/mongodb/mongo-go-driver/mongo"
)

type App struct {
	Router     *mux.Router
	DB         *sql.DB //db handler
	Controller *DeploymentEngineController
}

func (a *App) Initialize(user, password, dbname string) {
	connectionString := fmt.Sprintf("%s:%s@tcp(mysql:3306)/%s", user, password, dbname) //root:root@/k8sql

	var err error
	a.DB, err = sql.Open("mysql", connectionString)
	if err != nil {
		//log.Fatal(err)
	}

	a.Router = mux.NewRouter()
	a.initializeRoutes()
	a.createDB(a.DB)

	client, err := mongo.NewClient("mongodb://mongo:27017")
	if err == nil {
		err = client.Connect(context.Background())
		if err == nil {
			db := client.Database("deployment_engine")
			if db != nil {
				controller := DeploymentEngineController{
					collection: db.Collection("deployments"),
				}
				a.Controller = &controller
			}
		} else {
			fmt.Printf("Error connecting to MongoDB: %s", err.Error())
		}
	} else {
		fmt.Printf("Error getting client for MongoDB: %s", err.Error())
	}

}

func (a *App) Run(addr string) {
	log.Fatal(http.ListenAndServe(addr, a.Router))
}

func (a *App) initializeRoutes() {
	a.Router.HandleFunc("/deps", a.getDeps).Methods("GET")
	a.Router.HandleFunc("/dep", a.createDep).Methods("POST")
	a.Router.HandleFunc("/dep/{id}", a.getDep).Methods("GET")
	a.Router.HandleFunc("/dep/{id}", a.deleteDep).Methods("DELETE")
}

func (a *App) createDB(db *sql.DB) error {
	statement := fmt.Sprintf("SELECT 1 FROM deploymentsBlueprint LIMIT 1") //most simple check if exists
	_, err := db.Query(statement)
	if err != nil {
		fmt.Println("Creating deploymentsBlueprint table")
		statement = fmt.Sprintf("CREATE TABLE deploymentsBlueprint ( id VARCHAR(50) PRIMARY KEY, description VARCHAR(50) NOT NULL, status VARCHAR(50) NOT NULL, type VARCHAR(50), api_endpoint VARCHAR(50), api_type VARCHAR(50), keypair_id VARCHAR(50) )")
		_, err = db.Exec(statement)
	}

	statement = fmt.Sprintf("SELECT 1 FROM nodesBlueprint LIMIT 1")
	_, err = db.Query(statement)
	if err != nil {
		fmt.Println("Creating nodesBlueprint table")
		statement = fmt.Sprintf("CREATE TABLE nodesBlueprint ( id VARCHAR(50) PRIMARY KEY, dep_id VARCHAR(50), region VARCHAR(50), public_ip VARCHAR(50), role VARCHAR(50), ram INT, cpu INT, status VARCHAR(50), type VARCHAR(50), disc VARCHAR(50), generate_ssh_keys VARCHAR(50), ssh_keys_id VARCHAR(50), baseimage VARCHAR(50), arch VARCHAR(50), os VARCHAR(50), INDEX d_id (dep_id), FOREIGN KEY (dep_id)  REFERENCES deploymentsBlueprint(id)  ON DELETE CASCADE )")
		_, err = db.Exec(statement)
	}
	return err
}

func (a *App) getDeps(w http.ResponseWriter, r *http.Request) {
	count, _ := strconv.Atoi(r.FormValue("count"))
	start, _ := strconv.Atoi(r.FormValue("start"))

	if count > 100 || count < 1 {
		count = 100 //in case of 100+ deployments, just remove it, safety valve
	}
	if start < 0 {
		start = 0
	}

	products, err := getDeps(a.DB, start, count)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, products)
}

func (a *App) createDep(w http.ResponseWriter, r *http.Request) {
	var bp blueprint.BlueprintType
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&bp); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}
	defer r.Body.Close()

	if err := a.Controller.createDep(bp); err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondWithJSON(w, http.StatusCreated, bp)
	//respondWithJSON(w, http.StatusOK, map[string]string{"result": "success"})
}

func (a *App) getDep(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	u := dep{Id: id}
	if err := u.getDep(a.DB); err != nil {
		switch err {
		case sql.ErrNoRows:
			respondWithError(w, http.StatusNotFound, "Dep not found")
		default:
			respondWithError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	if err := u.getNodes(a.DB); err != nil {
		switch err {
		case sql.ErrNoRows:
			respondWithError(w, http.StatusNotFound, "Nodes not found")
		default:
			respondWithError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	respondWithJSON(w, http.StatusOK, u)
}

func (a *App) deleteDep(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	u := dep{Id: id}
	if err := u.getDep(a.DB); err != nil {
		switch err {
		case sql.ErrNoRows:
			respondWithError(w, http.StatusNotFound, "Dep not found")
		default:
			respondWithError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	if err := u.getNodes(a.DB); err != nil {
		switch err {
		case sql.ErrNoRows:
			respondWithError(w, http.StatusNotFound, "Nodes not found")
		default:
			respondWithError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	//
	if err := u.deleteDep(a.DB); err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, map[string]string{"result": "success"})
}

func respondWithError(w http.ResponseWriter, code int, message string) {
	respondWithJSON(w, code, map[string]string{"error": message})
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, _ := json.Marshal(payload)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(response)
}
