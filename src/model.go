// model.go

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	blueprint "github.com/DITAS-Project/blueprint-go"
	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
)

const (
	errorStatus         = "error"
	runningStatus       = "running"
	deletingStatus      = "deleting"
	componentNameRegexp = "[a-z]([-a-z0-9]*[a-z0-9])?"
)

type DeploymentEngineController struct {
	collection *mgo.Collection
}

type NodeInfo struct {
	Name string `json:"name"`
	IP   string `json:"ip"`
}

type Deployment struct {
	ID        string                  `json:"id" bson:"_id"`
	Blueprint blueprint.BlueprintType `json:"blueprint"`
	Nodes     []NodeInfo              `json:"nodes"`
	MasterIP  string                  `json:"master_ip" bson:"master_ip"`
	NumVDCs   int                     `json:"num_vdcs" bson:"num_vdcs"`
	Status    string                  `json:"status"`
	VDCs      []string                `json:"vdcs"`
}

func sanitize(name string) (string, error) {
	replaced := strings.Replace(name, "_", "-", -1)
	valid, err := regexp.Match(componentNameRegexp, []byte(replaced))
	if !valid {
		return "", fmt.Errorf("Sanitized blueprint name %s is not valid for kubernetes deployment", replaced)
	}
	return replaced, err
}

func executeCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (c *DeploymentEngineController) deleteDeployment(bpName string, deployment Deployment) error {
	var pythonArgs []string
	for _, inf := range deployment.Blueprint.CookbookAppendix.Infrastructure {
		for _, node := range inf.Resources {
			fmt.Println(node.Name)
			pythonArgs = append(pythonArgs, node.Name)
		}
	}
	// update database with deployment status - deleting
	c.setStatus(bpName, deletingStatus)

	fmt.Println("\nGO: Calling python script to remove old deployment:", bpName)
	err := executeCommand("kubernetes/delete_vm.py", pythonArgs...)
	if err != nil {
		fmt.Println(err.Error())
		return err
	}
	err = c.collection.RemoveId(bpName)
	fmt.Println("GO: Finished")
	return err
}

func (c *DeploymentEngineController) DeleteVDC(bpName string, vdcId string, deleteDeployment bool) error {
	deployment, err := c.findDeployment(bpName)
	if err == nil {
		if vdcId != "" {
			//TODO: Remove VDC
		}

		if deleteDeployment && (vdcId == "" || len(deployment.VDCs) == 0) {
			return c.deleteDeployment(bpName, deployment)
		}

		return nil
	}
	return err
}

func (c *DeploymentEngineController) updateDeployment(bpName string, update bson.M) {
	err := c.collection.UpdateId(bpName, update)
	if err != nil {
		fmt.Printf("Error updating blueprint %s status to %v: %s", bpName, update, err.Error())
	}
}

func (c *DeploymentEngineController) setStatus(bpName string, status string) {
	c.updateDeployment(bpName, bson.M{"$set": bson.M{"status": status}})

}

func (c *DeploymentEngineController) findDeployment(bpName string) (Deployment, error) {
	var deployment Deployment
	err := c.collection.FindId(bpName).One(&deployment)
	return deployment, err
}

func (c *DeploymentEngineController) getNodeIps() (map[string]string, error) {
	result := make(map[string]string)
	file, err := os.Open("kubernetes/inventory")
	defer file.Close()
	if err != nil {
		return result, err
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		text := scanner.Text()
		if strings.Index(text, "[") != 0 {
			tokens := strings.Split(text, " ")
			if len(tokens) > 1 {
				host := tokens[0]
				hostInfo := tokens[1]
				hostInfoTokens := strings.Split(hostInfo, "=")
				if len(hostInfoTokens) > 1 && hostInfoTokens[0] == "ansible_ssh_host" {
					result[host] = hostInfoTokens[1]
				} else {
					fmt.Printf("Invalid ansible_ssh_host found in inventory for host %s: %s\n", host, hostInfo)
				}
			} else {
				fmt.Printf("Invalid host info line found in inventory: %s\n", text)
			}
		}
	}

	if scanner.Err() != nil {
		return result, scanner.Err()
	}

	return result, nil

}

func (c *DeploymentEngineController) CreateDep(bp blueprint.BlueprintType) error {
	collection := c.collection
	bpName := *bp.InternalStructure.Overview.Name
	bpNameSanitized, err := sanitize(bpName)
	if err != nil {
		return err
	}
	deployment, err := c.findDeployment(bpName)
	if err != nil {
		deployment := Deployment{
			Blueprint: bp,
			ID:        *bp.InternalStructure.Overview.Name,
			NumVDCs:   0,
			Status:    "starting",
		}
		err = collection.Insert(deployment)
		var masterName string
		if err == nil {
			for _, infra := range bp.CookbookAppendix.Infrastructure {
				pythonArgs := make([]string, 0, len(infra.Resources)*3)
				for _, node := range infra.Resources {
					if strings.ToLower(node.Role) == "master" {
						masterName = node.Name
					}
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

				nodeIps, err := c.getNodeIps()
				if err != nil {
					fmt.Printf("Error reading node ips: %s\n", err.Error())
					c.setStatus(bpName, errorStatus)
					return err
				}

				var masterIp string
				nodes := make([]NodeInfo, 0, len(nodeIps))
				for name, ip := range nodeIps {
					if name == masterName {
						masterIp = ip
					}
					nodes = append(nodes, NodeInfo{
						Name: name,
						IP:   ip,
					})
				}

				c.updateDeployment(bpName, bson.M{
					"$set": bson.M{"master_ip": masterIp, "nodes": nodes},
				})

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
				vars := fmt.Sprintf("blueprintName=%s vdmName=%s", bpName, bpNameSanitized)
				err2 := executeCommand("ansible-playbook", "kubernetes/ansible_deploy.yml", "--inventory=kubernetes/inventory", "--extra-vars", vars)

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

	vdcNumber := deployment.NumVDCs
	vdcName := fmt.Sprintf("%s%d", bpNameSanitized, vdcNumber)

	fmt.Printf("\nGO: Calling Ansible to add VDC %d", vdcNumber)
	//time.Sleep(20 * time.Second) //safety valve in case of one command after another
	vars := fmt.Sprintf("blueprintName=%s vdcName=%s", bpName, vdcName)
	err2 := executeCommand("ansible-playbook", "kubernetes/ansible_deploy_add.yml", "--inventory=kubernetes/inventory", "--extra-vars", vars)

	if err2 != nil {
		fmt.Println(err2.Error())
		c.setStatus(bpName, errorStatus)
		return err2
	}

	c.updateDeployment(bpName, bson.M{
		"$inc":  bson.M{"num_vdcs": 1},
		"$push": bson.M{"vdcs": vdcName},
	})

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

func (c *DeploymentEngineController) GetAllDeps() ([]Deployment, error) {
	var result []Deployment
	err := c.collection.Find(nil).All(&result)
	return result, err
}

func (c *DeploymentEngineController) GetDep(id string) (Deployment, error) {
	return c.findDeployment(id)
}
