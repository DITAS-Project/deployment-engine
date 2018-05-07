## Deployment engine for kubernetes cluster

The project consists of elements:
1. REST API in GOlang allowing an upload, viewing and removal of deployments

2. Local MySQL database to store the deployments and nodes information

3. Virtual machine creation and removal scripts in Python to access CloudSigma resources

4. Ansible playbook used on the created virtual machine to set up a kubernetes cluster

Data is structured as in the following schema - swagger api file:
https://app.swaggerhub.com/apis/jacekwachowiak/REST-Kubernetes/1.0.0

Steps to go:
* rewrite scripts if multiple masters are needed, add information about the status of the deployment to the python return arguments
* rewrite swagger file and all dependencies in the code to match the blueprint

To run the project in its current state it is necessary to have a working instance of MySQL (for ubuntu `sudo apt-get install mysql-server`, user/pwd used are `root/root`), 
go (with libraries - when you build for the first time `go` will tell you what is missing and you can get it with `go get x` where `x` is usually a package in form of `github.com/...`), 
curl and python 2.7 (with libraries - use `pip install` to get them: `cloudsigma`, `mysqlclient`) installed and use multiple command lines (at least 2).

It is assumed that the user can get access to the database with credentials root/root. To set up the database log into it with `mysql -u root -p` and input the password - equal to the username - `root`. In other case, code in `main.go` has to be edited.

Next a database must be created (only once) - `CREATE DATABASE k8sql;` and switched on with - 
`USE k8sql;` All other operations are done automatically.

To compile go code run `go build` in `/k8sql/`. To run it, run the binary `./deployment-engine` from the same location.
This will lock the command line. Next commands must be run in a new window.

To add a deployment `test` use curl - `curl -H "Content-Type: application/json" -d '{"id":"test", "name":"AddedDep", "status":"starting", "nodes": [{"id": "sth1", "region": "ZRH", "public_ip": "168.192.0.1", "role": "none", "ram":2048, "cpu":2000, "status":"starting"}, {"id": "sth2", "region": "MIA", "public_ip": "168.192.0.2", "role": "none", "ram":1024, "cpu":2000, "status":"starting"}]}' http://localhost:8080/dep
` - this will create a test deployment with 2 nodes. It takes time to set everything up.

To view all deployments go to your browser and check `localhost:8080/deps`, to check a specific deployment check `localhost:8080/dep/test` where test is the name of the deployment.

To check from the command line type `curl http://localhost:8080/deps` for all and `curl http://localhost:8080/dep/test` for a specific one.

To remove a deployment called `test` run `curl -X DELETE http://localhost:8080/dep/test`

Remember that the name of the deployment and names of nodes are unique. Depending on the available resources more nodes can be added. The example works with 2 - one master and one slave. Parameters such as RAM, CPU are also modifiable.
Parameters such as IP address, status are downloaded from CloudSigma servers and set in the database by python script.

