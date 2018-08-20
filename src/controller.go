// model.go

package main

import (
	"deployment-engine/src/cloudsigma"
	"deployment-engine/src/ditas"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strings"

	blueprint "github.com/DITAS-Project/blueprint-go"
	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"

	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

const (
	errorStatus         = "error"
	runningStatus       = "running"
	deletingStatus      = "deleting"
	componentNameRegexp = "[a-z]([-a-z0-9]*[a-z0-9])?"
)

type DeploymentEngineController struct {
	collection *mgo.Collection
	homedir    string
}

func sanitize(name string) (string, error) {
	replaced := strings.Replace(name, "_", "-", -1)
	replaced = strings.Replace(replaced, " ", "-", -1)
	valid, err := regexp.Match(componentNameRegexp, []byte(replaced))
	if !valid {
		return "", fmt.Errorf("Sanitized blueprint name %s is not valid for kubernetes deployment", replaced)
	}
	return replaced, err
}

func executeCommand(logger *log.Entry, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = logger.Writer()
	cmd.Stderr = logger.Writer()
	return cmd.Run()
}

func (c *DeploymentEngineController) deleteDeployment(bpName string, deployment ditas.InfrastructureDeployment) error {
	/*var pythonArgs []string
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
	fmt.Println("GO: Finished")*/
	return nil
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

func (c *DeploymentEngineController) findDeployment(bpName string) (ditas.InfrastructureDeployment, error) {
	var deployment ditas.InfrastructureDeployment
	err := c.collection.FindId(bpName).One(&deployment)
	return deployment, err
}

/*func (c *DeploymentEngineController) getNodeIps() (map[string]string, error) {
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

}*/

func writeHost(node ditas.NodeInfo, file *os.File) (int, error) {
	line := fmt.Sprintf("%s ansible_ssh_host=%s ansible_ssh_user=%s\n", node.Name, node.IP, node.Username)
	return file.WriteString(line)
}

func (c *DeploymentEngineController) createInventory(logger *log.Entry, bpID string, deployment ditas.InfrastructureDeployment) error {
	path := "kubernetes/" + bpID
	filePath := path + "/inventory"
	logger.Infof("Creating inventory at %s", filePath)
	inventory, err := os.Create(filePath)
	defer inventory.Close()

	if err != nil {
		logger.WithError(err).Errorf("Error creating inventory file %s", filePath)
		return err
	}

	_, err = inventory.WriteString("[master]\n")
	if err != nil {
		logger.WithError(err).Error("Error writing master header to inventory")
		return err
	}

	_, err = writeHost(deployment.Master, inventory)
	if err != nil {
		logger.WithError(err).Error("Error writing master information to inventory")
		return err
	}

	_, err = inventory.WriteString("[slaves]\n")
	if err != nil {
		logger.WithError(err).Error("Error writing slaves header to inventory")
		return err
	}
	for _, slave := range deployment.Slaves {
		_, err = writeHost(slave, inventory)
		if err != nil {
			logger.WithError(err).Errorf("Error writing slave %s header to inventory", slave.Name)
			return err
		}
	}

	logger.Info("Inventory correctly created")

	return nil
}

func (c *DeploymentEngineController) writeBlueprint(logger *log.Entry, bp blueprint.BlueprintType, bpID string) error {
	path := "kubernetes/" + bpID
	name := path + "/blueprint.json"
	logger.Infof("Copying blueprint to %s", name)
	err := os.MkdirAll(path, os.ModePerm)
	if err != nil {
		logger.WithError(err).Errorf("Error creating inventory folder %s", path)
		return err
	}

	jsonData, err := json.Marshal(bp)
	jsonFile, err := os.Create(name)
	if err != nil {
		logger.WithError(err).Errorf("Error creating blueprint file %s", name)
		return err
	}
	defer jsonFile.Close()
	_, err = jsonFile.Write(jsonData)
	if err != nil {
		logger.WithError(err).Errorf("Error writing blueprint file %s", name)
		return err
	}

	logger.Info("Blueprint copied")

	return nil
}

func (c *DeploymentEngineController) deployK8s(logger *log.Entry, bpId string, deployment ditas.InfrastructureDeployment) error {
	err := c.createInventory(logger, bpId, deployment)
	if err != nil {
		return err
	}

	logger.Info("Calling Ansible for initial k8s deployment")
	//time.Sleep(180 * time.Second)
	vars := fmt.Sprintf("blueprintName=%s", bpId)
	inventory := fmt.Sprintf("--inventory=kubernetes/%s/inventory", bpId)
	err2 := executeCommand(logger, "ansible-playbook", "kubernetes/ansible_deploy.yml", inventory, "--extra-vars", vars)

	if err2 != nil {
		logger.WithError(err).Error("Error executing ansible deployment for k8s deployment")
		return err2
	}

	logger.Info("k8s cluster created!!!!")
	return nil
}

func (c *DeploymentEngineController) verifySshHost(logInput *log.Entry, node ditas.NodeInfo, config *ssh.ClientConfig) error {
	logger := logInput.WithField("host", node.IP)
	logger.Info("Verifying host ssh availability")

	_, err := ssh.Dial("tcp", node.IP+":22", config)
	if err != nil {
		logger.WithError(err).Errorf("Failed to dial: " + err.Error())
		return err
	}

	logger.Info("SSH Verified for host")
	return nil
}

func (c *DeploymentEngineController) addHostCallback(hostname string, remote net.Addr, key ssh.PublicKey) error {
	logger := log.WithField("host", remote.String())
	logger.Info("Adding host to known_hosts")
	line := knownhosts.Line([]string{knownhosts.Normalize(remote.String())}, key)
	return ioutil.WriteFile(c.homedir+"/.ssh/known_hosts", []byte(line), os.ModeAppend)
}

func (c *DeploymentEngineController) verifySsh(logger *log.Entry, infra ditas.InfrastructureDeployment) error {

	logger.Info("Verifying SSH availability")
	key, err := ioutil.ReadFile(c.homedir + "/.ssh/id_rsa")
	if err != nil {
		log.WithError(err).Error("Error reading keystore")
		return err
	}

	// Create the Signer for this private key.
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		log.WithError(err).Error("Error parsing private key")
		return err
	}

	config := &ssh.ClientConfig{
		User: "cloudsigma",
		Auth: []ssh.AuthMethod{
			// Use the PublicKeys method for remote authentication.
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: c.addHostCallback,
	}

	err = c.verifySshHost(logger, infra.Master, config)
	if err != nil {
		logger.WithError(err).Error("Error validating ssh on the master")
		return err
	}
	for _, resource := range infra.Slaves {
		err = c.verifySshHost(logger, resource, config)
		if err != nil {
			logger.WithError(err).Errorf("Error validating ssh on the slave %s", resource.Name)
			return err
		}
	}
	logger.Info("SSH availability verified")
	return err
}

func (c *DeploymentEngineController) addHostToHostFile(logger *log.Entry, hostInfo ditas.NodeInfo) error {
	host := "%s@%s"
	command := "echo %s %s | sudo tee -a /etc/hosts > /dev/null 2>&1"
	return executeCommand(logger, "ssh", "-o \"StrictHostKeyChecking no\"",
		fmt.Sprintf(host, hostInfo.Username, hostInfo.IP),
		fmt.Sprintf(command, hostInfo.Name, hostInfo.IP))
}

func (c *DeploymentEngineController) addToHostFile(logger *log.Entry, infra ditas.InfrastructureDeployment) error {

	err := c.addHostToHostFile(logger, infra.Master)

	if err != nil {
		logger.WithError(err).Error("Error adding master to hosts")
		return err
	}

	for _, slave := range infra.Slaves {
		err = c.addHostToHostFile(logger, slave)
		if err != nil {
			logger.WithError(err).Errorf("Error adding slave %s to hosts", slave.Name)
			return err
		}
	}

	return nil
}

func (c *DeploymentEngineController) CreateDep(bp blueprint.BlueprintType) error {

	bpName := *bp.InternalStructure.Overview.Name
	logger := log.WithField("blueprint", bpName)
	bpNameSanitized, err := sanitize(bpName)
	if err != nil {
		logger.Errorf("Error sanitizing blueprint name: %s", err.Error())
		return err
	}

	deployment := ditas.Deployment{
		ID:              bpNameSanitized,
		Blueprint:       bp,
		Infrastructures: make([]ditas.InfrastructureDeployment, len(bp.CookbookAppendix.Infrastructure)),
		Status:          "starting",
	}
	err = c.collection.Insert(deployment)
	if err != nil {
		logger.WithError(err).Error("Error inserting deployment in the database")
		return err
	}

	var deployer ditas.Deployer
	for i, infra := range bp.CookbookAppendix.Infrastructure {
		if strings.ToLower(infra.APIType) == "cloudsigma" {
			deployer, err = cloudsigma.NewDeployer()
			if err != nil {
				fmt.Printf("Error creating cloudsigma deployer: %s\n", err.Error())
				return err
			}
			infraDeployment, err := deployer.DeployInfrastructure(infra, bpNameSanitized)
			if err == nil {
				deployment.Infrastructures[i] = infraDeployment
				err = c.collection.UpdateId(deployment.ID, deployment)
				if err != nil {
					logger.WithError(err).Error("Error updating deployment status")
				}

				err = c.addToHostFile(logger, infraDeployment)
				if err != nil {
					logger.WithError(err).Error("SSH is not available")
					return err
				}

				err = c.writeBlueprint(logger, bp, bpNameSanitized)
				if err != nil {
					logger.WithError(err).Error("Error writing blueprint")
					return err
				}

				err = c.deployK8s(logger, bpNameSanitized, infraDeployment)
				if err != nil {
					logger.WithError(err).Error("Error deploying kubernetes cluster")
					return err
				}
			}
		}
	}
	return nil
}

func (c *DeploymentEngineController) CreateDepOld(bp blueprint.BlueprintType) error {
	/*	collection := c.collection
		bpName := *bp.InternalStructure.Overview.Name
		bpNameSanitized, err := sanitize(bpName)
		if err != nil {
			return err
		}
		deployment, err := c.findDeployment(bpName)
		if err != nil {
			deployment := ditas.InfrastructureDeployment{
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
					nodes := make([]ditas.NodeInfo, 0, len(nodeIps))
					for name, ip := range nodeIps {
						if name == masterName {
							masterIp = ip
						}
						nodes = append(nodes, ditas.NodeInfo{
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

		fmt.Println("GO: Finished")*/

	return nil
}

func (c *DeploymentEngineController) GetAllDeps() ([]ditas.InfrastructureDeployment, error) {
	var result []ditas.InfrastructureDeployment
	err := c.collection.Find(nil).All(&result)
	return result, err
}

func (c *DeploymentEngineController) GetDep(id string) (ditas.InfrastructureDeployment, error) {
	return c.findDeployment(id)
}
