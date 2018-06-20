// model.go

package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"
)

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
	var item node //2
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
	out, err := exec.Command("kubernetes/delete_vm.py", pythonArgs...).Output()
	fmt.Print(string(out))
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

func (u *dep) createDep(db *sql.DB) error {
	status := "starting"
	default_ip := "assigning"
	region := "default"
	statement := fmt.Sprintf("INSERT INTO deploymentsBlueprint(id, description, status, type, api_endpoint, api_type, keypair_id) VALUES('%s', '%s', '%s', '%s', '%s', '%s', '%s')", u.Id, u.Description, status, u.Type, u.Api_endpoint, u.Api_type, u.Keypair_id)
	_, err := db.Exec(statement)
	if err != nil {
		//here json file is created
		u.getDep(db)
		u.getNodes(db)
		jsonData, _ := json.Marshal(u)
		name := "./blueprint" + strconv.Itoa(BlueprintCount) + ".json"
		jsonFile, err := os.Create(name)
		if err != nil {
			panic(err)
		}
		defer jsonFile.Close()
		jsonFile.Write(jsonData)
		jsonFile.Close()
		fmt.Println("\nGO: Calling Ansible to add more components")
		time.Sleep(20 * time.Second) //safety valve in case of one command after another
		cmd := exec.Command("ansible-playbook", "kubernetes/ansible_deploy_add.yml", "--inventory=kubernetes/inventory", "--extra-vars", "blueprintNumber="+strconv.Itoa(BlueprintCount))
		out2, err2 := cmd.Output()
		//log file
		log, err := os.Create("log2.txt")
		if err != nil {
			panic(err)
		}
		log.WriteString("2 log for ansible \n")
		log.WriteString(string(out2))
		log.Close()
		//
		fmt.Print(string(out2))
		if err2 != nil {
			fmt.Println(err2.Error())
			return err2
		}
		BlueprintCount++
		return nil
	}
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
	fmt.Println("\nGO: Calling python script with arguments below: ")
	fmt.Println(pythonArgs)
	out, err := exec.Command("kubernetes/create_vm.py", pythonArgs...).Output()
	fmt.Print(string(out))
	if err != nil {
		fmt.Println(err.Error())
		return err
	}
	//here json file is created
	u.getDep(db)
	u.getNodes(db)
	jsonData, _ := json.Marshal(u)
	name := "./blueprint" + strconv.Itoa(BlueprintCount) + ".json"
	jsonFile, err := os.Create(name)
	if err != nil {
		panic(err)
	}
	defer jsonFile.Close()
	jsonFile.Write(jsonData)
	jsonFile.Close()
	//here after successful python call, ansible playbook is run, at least 30s of pause is needed for a node (experimental)
	//80 seconds failed, try with 180 to be safe
	fmt.Println("\nGO: Calling Ansible")
	time.Sleep(180 * time.Second)
	cmd := exec.Command("ansible-playbook", "kubernetes/ansible_deploy.yml", "--inventory=kubernetes/inventory", "--extra-vars", "blueprintNumber="+strconv.Itoa(BlueprintCount))
	out2, err2 := cmd.Output()
	//log file
	log, err := os.Create("log.txt")
	if err != nil {
		panic(err)
	}
	log.WriteString("Log for ansible \n")
	log.WriteString(string(out2))
	log.Close()
	//
	fmt.Print(string(out2))
	if err2 != nil {
		fmt.Println(err2.Error())
		return err2
	}
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
	}

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
