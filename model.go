// model.go

package main

import (
	"database/sql"
	"fmt"
)

type node struct {
	Id        string `json:"id"`
	Region    string `json:"region"`
	Public_ip string `json:"public_ip"`
	Role      string `json:"role"`
	RAM       int    `json:"ram"`
	Cores     int    `json:"cores"`
	Status    string `json:"status"`
}
type dep struct {
	Id     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Nodes  []node `json:"nodes"`
}

func (u *dep) getDep(db *sql.DB) error {
	statement := fmt.Sprintf("SELECT id, name, status FROM deployments WHERE id='%s'", u.Id)
	return db.QueryRow(statement).Scan(&u.Id, &u.Name, &u.Status)

}

func (u *dep) getNodes(db *sql.DB) error {
	u.Nodes = make([]node, 0)                                                                                     //2
	statement := fmt.Sprintf("SELECT id, region, public_ip, role, ram, cores FROM nodes WHERE dep_id='%s'", u.Id) //
	rows, err := db.Query(statement)
	if err != nil {
		return err
	}
	index := 0
	var item node //2
	defer rows.Close()
	for rows.Next() {
		//rows.Scan(&u.Nodes[index].Id, &u.Nodes[index].Region, &u.Nodes[index].Public_ip, &u.Nodes[index].Role, &u.Nodes[index].RAM, &u.Nodes[index].Cores)
		rows.Scan(&item.Id, &item.Region, &item.Public_ip, &item.Role, &item.RAM, &item.Cores)
		index++
		u.Nodes = append(u.Nodes, item)

	}
	return nil
}

func (u *dep) deleteDep(db *sql.DB) error {
	statement := fmt.Sprintf("DELETE FROM deployments WHERE id='%s'", u.Id)
	_, err := db.Exec(statement)
	return err
}

func (u *dep) createDep(db *sql.DB) error {
	statement := fmt.Sprintf("INSERT INTO deployments(id, name, status) VALUES('%s', '%s', '%s')", u.Id, u.Name, u.Status)
	_, err := db.Exec(statement)
	for _, element := range u.Nodes {
		statement = fmt.Sprintf("INSERT INTO nodes(id, dep_id, region, public_ip, role, ram, cores) VALUES('%s', '%s', '%s', '%s', '%s', '%d', '%d')", element.Id, u.Id, element.Region, element.Public_ip, element.Role, element.RAM, element.Cores)
		_, err = db.Exec(statement)
	}

	err = db.QueryRow("SELECT LAST_INSERT_Id()").Scan(&u.Id) //check

	if err != nil {
		return err
	}

	return nil
}

func getDeps(db *sql.DB, start, count int) ([]dep, error) {
	statement := fmt.Sprintf("SELECT id, name, status FROM deployments LIMIT %d OFFSET %d", count, start)
	rows, err := db.Query(statement)

	if err != nil {
		return nil, err
	}
	defer rows.Close()
	deps := []dep{}
	for rows.Next() {
		var u dep
		if err := rows.Scan(&u.Id, &u.Name, &u.Status); err != nil {
			return nil, err
		}

		u.getNodes(db) //recursive

		deps = append(deps, u)
	}
	return deps, nil
}
