# Deployment engine for kubernetes cluster

This is a WIP - work in progress readme of a docker-based version of the deployment engine,
for a stable local version please refer to the last local commit from the repository:
https://github.com/DITAS-Project/deployment-engine/commit/d062be68ae4ed9c7b6c28227eb61e194a8a733d6
and to the corresponding README.

### Structure
The project consists of two docker containers, where the first one contains:
1. REST API in GOlang allowing an upload, viewing and removal of deployments

2. Virtual machine creation and removal scripts in Python to access CloudSigma resources

3. Ansible playbook used on the created virtual machine to set up a kubernetes cluster

And the other one is a MySQL database to store the deployments and nodes information

Data is structured as in the following schema - swagger api file (available in /src/api/):
https://app.swaggerhub.com/apis/jacekwachowiak/REST-Kubernetes/1.0.0

###Steps to go:
* rewrite scripts if multiple masters are needed, add information about the status of the deployment to the python return arguments
* rewrite swagger file and all dependencies in the code to match the blueprint if the blueprint final version is published

###Requirements
To run the project in its current state it is not necessary to have a working instance of MySQL,
neither go nor 
curl or python 2.7.
The principal component necessary to run the deployment is a working version of docker.

###Instructions
There are two dockerfiles. It is crucial to run the dockerfile for MySQL first as the connection between them is necessary.

####Create MySQL docker container
To begin go (`cd`) to `MySQL_docker` and run `docker build -t mysql/test .` to create the image. 
Next `docker run --detach --name=mysql --env="MYSQL_ROOT_PASSWORD=root" --publish 6603:3306  mysql/test` to run the image and create a container.

####Access the database (this section is optional)
If you want to enter the container type `docker exec -it mysql bash` - you will see the bash.
Then to access the database you should type `mysql -uroot -proot` where `root` is both a password and the user set while starting the image.
You can then see the database by selecting it `USE k8sql;` - the database is created automatically by a script given to dockerfile.
To see the tables you can run `SHOW tables;`, to see the content you can use `DESCRIBE x` where `x` is the table name.
For now there is nothing inside.

####Create Deployment-engine container
In a new window start again from `/deployment-engine`. Run `docker build -t deployment/test .` to build the deployment engine image.
To run it use `docker run -dit --name=test-deployment --link mysql:mysql deployment/test`. Notice the link - this container will be able to connect to the database no matter what IP it has and no matter if some of them suffer shutdowns.
To get inside the container run `docker exec -it test-deployment bash`.
Enter `bash` in two windows because the next step requires to run the app, which lock the command line after the command.
To run the app type `./src`. If no tables exist in MySQL database, they will be created.
If the app is running, you should go to the second bash window opened and add a deployment.

####Add a deployment
To add a deployment `test` use curl - 
`curl -H "Content-Type: application/json" -d '{"id":"test", "name":"AddedDep", "status":"starting", "nodes": [{"id": "sth1", "region": "ZRH", "public_ip": "168.192.0.1", "role": "none", "ram":2048, "cpu":2000, "status":"starting"}, {"id": "sth2", "region": "MIA", "public_ip": "168.192.0.2", "role": "none", "ram":1024, "cpu":2000, "status":"starting"}]}' http://localhost:8080/dep` - this will create a test deployment with 2 nodes. 
It takes time to set everything up. You will be informed every big step by the API. Because the GO program calls external scripts, 
all returned output will be shown at once and not line by line in the first, locked window, where you started the app.

####View the deployments
To view deployments from the command line type `curl http://localhost:8080/deps` for all and `curl http://localhost:8080/dep/test` for a specific one - `test` in this case.

####Remove the deployment
To remove a deployment called `test` run `curl -X DELETE http://localhost:8080/dep/test`
Remember to remove the deployment after you are done! Otherwise it will stay and take your resources until you do.

####Remove the docker containers
To stop and remove the docker containers run `docker rm -f test-deployment` and `docker rm -f mysql`
Do this every time you want to rebuild them. It stops and removes the image so that all changes done by you are included in the next `run` command.
####Final note
Remember that the name of the deployment and names of nodes are unique. Depending on the available resources more nodes can be added. The example works with 2 - one master and one slave. Parameters such as RAM, CPU are also modifiable - just change them in `curl`.
Parameters such as IP address, status are downloaded from CloudSigma servers and set in the database by python script.

