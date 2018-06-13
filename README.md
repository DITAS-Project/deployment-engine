# Deployment engine for kubernetes cluster

This is a WIP - work in progress readme of a docker-based version of the deployment engine. The deployment engine lives in a virtual machine created and maintained solely for this purpose at IP of 31.171.247.156 - port 50012 for remote acces to the app - REST api.

### Structure
The project consists of two docker containers, where the first one contains:
1. REST API in GOlang allowing an upload, viewing and removal of deployments

2. Virtual machine creation and removal scripts in Python to access CloudSigma resources

3. Ansible playbook used on the created virtual machine to set up a kubernetes cluster

And the other one is a MySQL database to store the deployments and nodes information
and is accessible through port 50013 at the same IP address.

Data is structured as in the following schema - swagger api file (available in /src/api/):
https://app.swaggerhub.com/apis/jacekwachowiak/REST-Kubernetes/1.0.0

### Steps to go:
* rewrite scripts if multiple masters are needed - abandoned for now, useful only with very large networks
* rewrite swagger file according to the blueprint - code was already updated
* handle deployment requests with automatic installing of the app inside the cluster - for now hardcoded SLA - see branch SLA of this repo

### Requirements
To run the project in its current state it is not necessary to have a working instance of MySQL,
neither go nor 
curl or python 2.7.
The principal component necessary to run the deployment is access to Jenkins platform. To make changes download the repository and add/commit/push.

### Instructions
There are two dockerfiles. 
One is used to build the application's executable - Dockerfile.build.
The other is an execution environment - Docker.artifact, where the app runs.
Every time there is a push/pull done, Jenkins will run the test on the repository. You can view the results at:
[http://178.22.71.23:8080/job/deployment-engine/job/master/](http://178.22.71.23:8080/job/deployment-engine/job/master/)

The next section assumes that we have a successful build running. To check that the engine is running go to:
[http://31.171.247.162:50012/deps](http://31.171.247.162:50012/deps), you should be able to load the page, no matter if there is any deployment already running. This is a version living on a Jenkins managed machine. To access an external one go to [http://31.171.247.156:50012/deps](http://31.171.247.156:50012/deps). Here every new version of docker must be pulled and rerun by hand but it allows to enter the container and all machines in the network directly.

#### Add a deployment
To add a deployment `test` use curl - 
`curl -H "Content-Type: application/json" -d '{"id":"test", "name":"AddedDep", "status":"starting", "nodes": [{"id": "sth1", "region": "ZRH", "public_ip": "168.192.0.1", "role": "none", "ram":2048, "cpu":2000, "status":"starting"}, {"id": "sth2", "region": "MIA", "public_ip": "168.192.0.2", "role": "none", "ram":1024, "cpu":2000, "status":"starting"}]}' 31.171.247.156:50012/dep` or change `156` to `162` but you will lose the possibility to enter to the cluster.
It takes time to set everything up. You will be informed by the API once the job is done or have failed. There is no intermediate output possible due to the fact that only a JSON can be returned and
all printing happens inside the container.

Having that, if you want to make sure that the machines were created correctly, you can refresh the status of the nodes when you view the deployment.
If they show `running` it means the python script has finished its job.

#### View the deployments
To view deployments from the command line type `curl 31.171.247.156:50012/deps` for all and `curl 31.171.247.156:50012/dep/test` for a specific one - `test` in this case.

#### Remove the deployment
To remove a deployment called `test` run `curl -X DELETE 31.171.247.156:50012/dep/test`
Remember to remove the deployment after you are done! Otherwise it will stay and take your resources until you do.

#### Final note
Remember that the name of the deployment and names of nodes are unique. Depending on the available resources more nodes can be added. The example works with 2 - one master and one slave. Parameters such as RAM, CPU are also modifiable - just change them in `curl`.
Parameters such as IP address, status are downloaded from CloudSigma servers and set in the database by python script.

The deployment engine works on an account with limited resources and with username and password saved privately so that the access is restricted to the members allowed to edit the core components.
