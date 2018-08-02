// model.go

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"

	blueprint "github.com/DITAS-Project/blueprint-go"
	bson "github.com/mongodb/mongo-go-driver/bson"
	mongo "github.com/mongodb/mongo-go-driver/mongo"
)

const (
	errorStatus   = "error"
	runningStatus = "running"
)

type DeploymentEngineController struct {
	collection *mongo.Collection
}

type Deployment struct {
	ID        string                  `json:"_id"`
	Blueprint blueprint.BlueprintType `json:"blueprint"`
	MasterIP  string                  `json:"master_ip"`
	NumVDCs   int                     `json:"num_vdcs"`
	Status    string                  `json:"status"`
}

type node struct {
	Id                string `json:"name"`
	Region            string `json:"region"`    //
	Public_ip         string `json:"public_ip"` //
	Role              string `json:"role"`
	RAM               int    `json:"ram"`
	Cpu               int    `json:"cpus"`
	Status            string `json:"status"` //
	Type              string `json:"type"`
	Disc              string `json:"disc"`
	Generate_ssh_keys string `json:"generate_ssh_keys"`
	Ssh_keys_id       string `json:"ssh_keys_id"`
	Base_image        string `json:"baseimage"`
	Arch              string `json:"arch"`
	Os                string `json:"os"`
}
type dep struct {
	Id           string `json:"name"`
	Description  string `json:"description"`
	Status       string `json:"on-line"`
	Type         string `json:"type"`
	Api_endpoint string `json:"api_endpoint"`
	Api_type     string `json:"api_type"`
	Keypair_id   string `json:"keypair_id"`
	Nodes        []node `json:"resources"`
}

func executeCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (u *dep) getDep(db *sql.DB) error {
	statement := fmt.Sprintf("SELECT id, description, status, type, api_endpoint, api_type, keypair_id FROM deploymentsBlueprint WHERE id='%s'", u.Id)
	return db.QueryRow(statement).Scan(&u.Id, &u.Description, &u.Status, &u.Type, &u.Api_endpoint, &u.Api_type, &u.Keypair_id)

}

func (u *dep) getNodes(db *sql.DB) error {
	u.Nodes = make([]node, 0)                                                                                                                                                                     //2
	statement := fmt.Sprintf("SELECT id, region, public_ip, role, ram, cpu, status, type, disc, generate_ssh_keys, ssh_keys_id, baseimage, arch, os FROM nodesBlueprint WHERE dep_id='%s'", u.Id) //
	rows, err := db.Query(statement)
	if err != nil {
		return err
	}
	index := 0
	var item node
	defer rows.Close()
	for rows.Next() {
		//rows.Scan(&u.Nodes[index].Id, &u.Nodes[index].Region, &u.Nodes[index].Public_ip, &u.Nodes[index].Role, &u.Nodes[index].RAM, &u.Nodes[index].Cpu)
		rows.Scan(&item.Id, &item.Region, &item.Public_ip, &item.Role, &item.RAM, &item.Cpu, &item.Status, &item.Type, &item.Disc, &item.Generate_ssh_keys, &item.Ssh_keys_id, &item.Base_image, &item.Arch, &item.Os)
		index++
		u.Nodes = append(u.Nodes, item)

	}
	return nil
}

func (u *dep) deleteDep(db *sql.DB) error {
	//here arguments for python are prepared
	var pythonArgs []string
	for _, element := range u.Nodes {
		fmt.Println(element.Id)
		pythonArgs = append(pythonArgs, element.Id)
	}
	// update database with deployment status - deleting
	status := "deleting"
	statement := fmt.Sprintf("UPDATE deploymentsBlueprint SET deploymentsBlueprint.status = \042%s\042 WHERE deploymentsBlueprint.id = \042%s\042", status, u.Id)
	_, err := db.Exec(statement)
	if err != nil {
		return err
	}
	fmt.Println("\nGO: Calling python script to remove old deployment:", u.Id)
	err = executeCommand("kubernetes/delete_vm.py", pythonArgs...)
	if err != nil {
		fmt.Println(err.Error())
		return err
	}
	//
	statement = fmt.Sprintf("DELETE FROM deploymentsBlueprint WHERE id='%s'", u.Id)
	_, err = db.Exec(statement)
	fmt.Println("GO: Finished")
	return err
}

func (c *DeploymentEngineController) updateDeployment(bpName string, update *bson.Document) {
	_, err := c.collection.UpdateOne(context.Background(), bson.NewDocument(
		bson.EC.String("_id", bpName),
	), update)
	if err != nil {
		fmt.Printf("Error updating blueprint %s status to %v: %s", bpName, update, err.Error())
	}
}

func (c *DeploymentEngineController) setStatus(bpName string, status string) {
	c.updateDeployment(bpName, bson.NewDocument(bson.EC.String("status", status)))
}

func (c *DeploymentEngineController) createDep(bp blueprint.BlueprintType) error {
	collection := c.collection
	bpName := *bp.InternalStructure.Overview.Name
	var deployment Deployment
	found := collection.FindOne(context.Background(), bson.NewDocument(
		bson.EC.String("_id", bpName),
	))
	err := found.Decode(deployment)
	if err != nil {
		deployment := Deployment{
			Blueprint: bp,
			ID:        *bp.InternalStructure.Overview.Name,
			NumVDCs:   0,
			Status:    "starting",
		}
		depSerial, err := json.Marshal(deployment)
		if err == nil {
			document, err := bson.ParseExtJSONObject(string(depSerial))
			if err == nil {
				_, err := collection.InsertOne(context.Background(), document)
				if err == nil {
					for _, infra := range bp.CookbookAppendix.Infrastructure {
						pythonArgs := make([]string, 0, len(infra.Resources)*3)
						for _, node := range infra.Resources {
							pythonArgs = append(pythonArgs, node.Name)
							pythonArgs = append(pythonArgs, node.RAM)
							pythonArgs = append(pythonArgs, node.CPUs)
						}

						fmt.Println("\nGO: Calling python script with arguments below: ")
						fmt.Println(pythonArgs)
						err := executeCommand("kubernetes/create_vm.py", pythonArgs...)
						if err != nil {
							fmt.Println(err.Error())
							c.setStatus(bpName, errorStatus)
							return err
						}

						jsonData, err := json.Marshal(bp)
						name := "./blueprint_" + bpName + ".json"
						jsonFile, err := os.Create(name)
						if err != nil {
							c.setStatus(bpName, errorStatus)
							panic(err)
						}
						defer jsonFile.Close()
						jsonFile.Write(jsonData)
						jsonFile.Close()

						//here after successful python call, ansible playbook is run, at least 30s of pause is needed for a node (experimental)
						//80 seconds failed, try with 180 to be safe
						fmt.Println("\nGO: Calling Ansible for initial k8s deployment")
						//time.Sleep(180 * time.Second)
						err2 := executeCommand("ansible-playbook", "kubernetes/ansible_deploy.yml", "--inventory=kubernetes/inventory", "--extra-vars", "blueprintName="+bpName)

						if err2 != nil {
							fmt.Println(err2.Error())
							c.setStatus(bpName, errorStatus)
							return err2
						}

						c.setStatus(bpName, runningStatus)
					}
				} else {
					fmt.Printf("Error inserting deployment into database: %s", err.Error())
				}
			}
		}
	}

	vdcNumber := deployment.NumVDCs

	fmt.Printf("\nGO: Calling Ansible to add VDC %d", vdcNumber)
	//time.Sleep(20 * time.Second) //safety valve in case of one command after another
	err2 := executeCommand("ansible-playbook", "kubernetes/ansible_deploy_add.yml", "--inventory=kubernetes/inventory", "--extra-vars", "blueprintName="+bpName+" "+"vdcNumber="+strconv.Itoa(vdcNumber))

	if err2 != nil {
		fmt.Println(err2.Error())
		c.setStatus(bpName, errorStatus)
		return err2
	}

	c.updateDeployment(bpName, bson.NewDocument(
		bson.EC.Int32("num_vdcs", int32(vdcNumber+1)),
	))

	fmt.Println("GO: Finished")
	/*status := "starting"
	default_ip := "assigning"
	region := "default"
	statement := fmt.Sprintf("INSERT INTO deploymentsBlueprint(id, description, status, type, api_endpoint, api_type, keypair_id) VALUES('%s', '%s', '%s', '%s', '%s', '%s', '%s')", u.Id, u.Description, status, u.Type, u.Api_endpoint, u.Api_type, u.Keypair_id)
	_, err := db.Exec(statement)
	if err == nil {
		var pythonArgs []string
		for _, element := range u.Nodes {
			statement = fmt.Sprintf("INSERT INTO nodesBlueprint(id, dep_id, region, public_ip, role, ram, cpu, status, type, disc, generate_ssh_keys, ssh_keys_id, baseimage, arch, os) VALUES('%s', '%s', '%s', '%s', '%s', '%d', '%d', '%s', '%s', '%s', '%s', '%s', '%s', '%s', '%s')", element.Id, u.Id, region, default_ip, element.Role, element.RAM, element.Cpu, status, element.Type, element.Disc, element.Generate_ssh_keys, element.Ssh_keys_id, element.Base_image, element.Arch, element.Os)
			_, err = db.Exec(statement)
			//here arguments for python are prepared - name/ram/cpu of nodesBlueprint are prepared
			pythonArgs = append(pythonArgs, element.Id)
			pythonArgs = append(pythonArgs, strconv.Itoa(element.RAM))
			pythonArgs = append(pythonArgs, strconv.Itoa(element.Cpu))
			//
		}

		//here json file is created
		u.getDep(db)
		u.getNodes(db)

	}

	fmt.Printf("\nGO: Calling Ansible to add VDC %d", BlueprintCount)
	//time.Sleep(20 * time.Second) //safety valve in case of one command after another
	err2 := executeCommand("ansible-playbook", "kubernetes/ansible_deploy_add.yml", "--inventory=kubernetes/inventory", "--extra-vars", "blueprintNumber="+strconv.Itoa(BlueprintCount))

	if err2 != nil {
		fmt.Println(err2.Error())
		return err2
	}
	BlueprintCount++

	BlueprintCount++
	// update database with deployment status - running
	status = "running"
	statement = fmt.Sprintf("UPDATE deploymentsBlueprint SET deploymentsBlueprint.status = \042%s\042 WHERE deploymentsBlueprint.id = \042%s\042", status, u.Id)
	_, err = db.Exec(statement)
	if err != nil {
		fmt.Println(err.Error())
		//return err
	}
	fmt.Println("GO: Finished")
	//
	err = db.QueryRow("SELECT LAST_INSERT_Id()").Scan(&u.Id) //check

	if err != nil {
		return err
	}*/

	return nil
}

func getDeps(db *sql.DB, start, count int) ([]dep, error) {
	statement := fmt.Sprintf("SELECT id, description, status, type, api_endpoint, api_type, keypair_id FROM deploymentsBlueprint LIMIT %d OFFSET %d", count, start)
	rows, err := db.Query(statement)

	if err != nil {
		return nil, err
	}
	defer rows.Close()
	deps := []dep{}
	for rows.Next() {
		var u dep
		if err := rows.Scan(&u.Id, &u.Description, &u.Status, &u.Type, &u.Api_endpoint, &u.Api_type, &u.Keypair_id); err != nil {
			return nil, err
		}

		u.getNodes(db) //recursive

		deps = append(deps, u)
	}
	return deps, nil
}
