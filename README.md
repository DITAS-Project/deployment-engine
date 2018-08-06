# Deployment engine for kubernetes cluster

This is a WIP - work in progress readme of a docker-based version of the deployment engine. The deployment engine lives in a virtual machine created and maintained solely for this purpose at IP of 31.171.247.156 - port 50012 for remote access to the app - REST api.

### Structure
The project consists of two docker containers, where the first one contains:
1. REST API in Golang allowing an upload, viewing and removal of deployments

2. Virtual machine creation and removal scripts in Python to access CloudSigma resources

3. Ansible playbook used on the created virtual machine to set up a kubernetes cluster

And the other one is a MongoDB database to store the deployments and nodes information
and is accessible through port 50014 at the same IP address.

Data is structured as in the following schema - swagger api file (available in /src/api/):
https://app.swaggerhub.com/apis/jacekwachowiak/REST-Kubernetes/1.1

### Steps to go

* rewrite scripts if multiple masters are needed - abandoned for now, useful only with very large networks

### Requirements

* Docker compose is the easiest way to run the project. To install it, please refer to the [documentation](https://docs.docker.com/compose/install/)
* A folder containing ssh keys in a .ssh folder and a file with CloudSigma credentials named .cloudsigma.conf. The default docker-compose script will consider it to be at `/home/cloudsigma/dep-engine-home` so if they are in any other location, the script should be modified.
* To make changes to the project download the repository and add/commit/push.

### Build Instructions

There are two dockerfiles.
One is used to build the application's executable - Dockerfile.build.
The other is an execution environment - Docker.artifact, where the app runs.
Every time there is a push/pull done, Jenkins will run the test on the repository. You can view the results at:
[http://178.22.71.23:8080/job/deployment-engine/job/master/](http://178.22.71.23:8080/job/deployment-engine/job/master/)

The next section assumes that we have a successful build running. To check that the engine is running go to:
[http://31.171.247.162:50012/deps](http://31.171.247.162:50012/deps), you should be able to load the page, no matter if there is any deployment already running. This is a version living on a Jenkins managed machine.

#### Running the deployment engine

* Create a folder in your local filesystem (i.e `$HOME/dep-engine-home`)
* Put your `.cloudsigma.conf` file with your CloudSigma credentials into that folder
* Create a `.ssh` subfolder and create a key-pair into it. For example `ssh-keygen -q -t rsa -N '' -f $HOME/dep-engine-home/.ssh/id_rsa`
* Download this Docker Compose [deployment file](https://raw.githubusercontent.com/DITAS-Project/deployment-engine/review_demo/docker-compose.yml) to any folder.
* Change the default `/home/cloudsigma/dep-engine-home` path to the path to the folder created in the first step.
* The deployment engine can be started by running `docker-compose up` in the folder that contains the Docker Compose file.
* To check that it's working, a `GET` request to `http://localhost:50012/deps` should return something.

#### Add a deployment

To add a deployment send a `POST` request to `http://localhost:50012/deps` with content type `application\json` header and a DITAS blueprint with a valid `COOKBOOK_APPENDIX` section with the description of the deployment as body of the request. You can do it with `curl` or with any other tool that's able to send arbitrary HTTP requests like [Postman](https://www.getpostman.com/).

It takes time to set everything up. You will be informed by the API once the job is done or have failed. There is no intermediate output possible due to the fact that only a JSON can be returned and
all printing happens inside the container.

Having that, if you want to make sure that the machines were created correctly, you can view the status of the nodes on the [http://localhost:50012/deps](http://localhost:50012/deps) page.
If the nodes show `running` it means the python script has finished its job and VM are created, as long as the full deployment is not `running` some other tasks, such as creating the cluster with ansible, are being performed.

Once the status is running, everything is set-up. All nodes are created and a VDM and VDC should be created for the DITAS blueprint. The id of this deployment is the name of this blueprint.

#### View the deployments

To view deployments from the command line type send a `GET` request to `http://localhost:50012/deps` such as `curl localhost:50012/deps` for all and `curl localhost:50012/deps/<blueprint_name>` for a specific one.

#### Rerun the deployment

The second time a blueprint is received, it won't launch a second infrastructure deployment. It will just create another instance of a VDC in the deployment already available. You can see the list of the VDC Identifiers in the deployment data.

#### Remove the deployment

To remove a deployment called `test` run `curl -X DELETE localhost:50012/deps/test?deleteDeployment=true`
Remember to remove the deployment after you are done! Otherwise it will stay and take your resources until you do.

#### Final note

Remember that the name of the deployment and names of nodes are unique. Depending on the available resources more nodes can be added. The example works with 2 - one master and one slave. Parameters such as RAM, CPU are also modifiable - just change them in the blueprint.
Parameters such as IP address, status are downloaded from CloudSigma servers and set in the database by python script.

The deployment engine works on an account with limited resources (4GHz and 9GB RAM left) and with username and password saved privately so that the access is restricted to the members allowed to edit the core components.

 For more commands please take a look at  [Useful commands for deployment engine.md](https://github.com/DITAS-Project/deployment-engine/blob/master/Useful%20commands%20for%20deployment%20engine.md).