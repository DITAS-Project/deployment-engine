# Installation instructions

## By source

- Make sure you have go version 1.11+ installed
- By default it will use MongoDB as the persistence repository. Unless you change it in the project's configuration, make sure that MongoDB is running and that it has the default configuration or provide the access URL with credentials in the `mongodb.url` property in the configuration file.
- By default it will use [Ansible](https://docs.ansible.com/ansible/latest/installation_guide/intro_installation.html) to provision the new deployments so **make sure that it's installed and configured** if you want to provision kubernetes and other products such as helm or glusterfs which depend on it.
- If you plan to provision other products over kubernetes such as traefik or rook, the [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/) command is required as well.
- Checkout the repository and make sure the root folder of the project is called `deployment-engine`.
- Execute `go build -o deployment-engine` in the root folder
- Read the [Configuration](#configuration) section for instructions about how to configure the different parameters of the deployment engine.
- Run the frontend by executing `./deployment-engine`. The server should be listening in port 8080 unless configured otherwise.

## Docker

- Make sure Docker is installed and running on your system
- Docker-compose is recommended to make running the engine easier
- Execute `docker build -t deployment-engine .`
- For easy execution we include a docker-compose deployment descriptor in `docker-compose.yml` that can be run. To do so:
- Create a folder in your machine and copy the docker-compose.yml file to it. 
- Create a subfolder named `config` and copy the provided `docker-compose/config.yml` file to it. You can personalize it as described in the [Configuration](#configuration) section. Don't modify the provided values except for the traefik section as they are tailored for the file layout present in the docker file.
- Create a subfolder named `ssh` and create a pair of ssh keys with the `ssh-keygen` command. The files must be named `id_rsa` and `id_rsa.pub` and the private key must be created with an empty passphrase.
- Run it with `docker-compose up`
- The default frontend should be available in port 8080 unless it was changed in the `docker-compose.yml` file

## Configuration

The deployment engine will look for a `~/deployment-engine-config/config.yml` file with the configuration. If it doesn't exist or it's empty, it will use some safe defaults. The properties that can be configured are:

### General configuration

- `repository.type`: The type of the persistence repository to use. By default it's `mongo` which will use MongoDB
- `provisioner.type`: The type of provisioner to use for new deployments. By default it's `ansible`
- `frontent.type`: The type of frontend that will be available. The default value `default` will start the default REST frontend described in the [usage instructuions](usage.md)

### MongoDB configuration

- `mongodb.url`: MongoDB URL to use for the persistence layer. By default it's `mongodb://localhost:27017` for local installation and `mongodb://mongo:27017` for docker
- `mongodb.vault.passphrase`: When using the vault functionality with mongoDB backend, this passphrase will be used to save the secret data encrypted into the database.

### Ansible configuration

- `ansible.folders.inventory`: Folder in which the deployment engine will store inventory information about deployments. It must be a folder writtable by the user which is running the application. By default it's `/tmp/ansible_inventories` although is **strongly** recommended to personalize this value if running locally. 
- `ansible.folders.scripts`: Folder containing the ansible scripts to deploy the different products. Some scripts are already provided in `provision/ansible` folder and that's the default value when running locally although it is **strongly** recommended too to provide a full path to this folder. When running in Docker this value will be automatically set.

## Usage 

Once installed, please read the [usage instructuions](usage.md)