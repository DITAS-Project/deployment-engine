# Installation instructions

## By source

- Make sure you have go version 1.11+ installed
- By default it will use MongoDB as the persistence repository. Unless you change it in the project's configuration, make sure that MongoDB is running and that it has the default configuration.
- By default it will use Ansible to provision the new deployments so make sure that it's installed and configured.
- Checkout the repository and make sure the root folder of the project is called `deployment-engine`.
- Execute `go build -o deployment-engine` in the root folder
- Run the frontend by executing `./deployment-engine`. The server should be listening in port 8080 unless configured otherwise.

## Docker

- Make sure Docker is installed and running on your system
- Docker-compose is recommended to make running the engine easier
- Execute `docker build -t deployment-engine .`
- For easy execution we include a docker-compose deployment descriptor in `docker-compose.yml` that can be run. To do so:
- Personalize the `docker-compose.yml` file, specially the volumes section with:
  - The path to the .cloudsigma.conf file in your environment if using the cloudsigma provider
  - The path to a .ssh folder containing the keys to use to manage deployments
- Run the containers with `docker-compose up`
- The default frontend should be available in port 8080 unless it was changed in the `docker-compose.yml` file

## Configuration

The deployment engine will look for a `~/deployment-engine/config.properties` file with the configuration. If it doesn't exist or it's empty, it will use some safe defaults. The properties that can be configured are:

### General configuration

- `repository.type`: The type of the persistence repository to use. By default it's `mongo` which will use MongoDB
- `provisioner.type`: The type of provisioner to use for new deployments. By default it's `ansible`
- `frontent.type`: The type of frontend that will be available. The default value `default` will start the default REST frontend described in the [usage instructuions](usage.md)

### MongoDB configuration

- `mongodb.url`: MongoDB URL to use for the persistence layer. By default it's `mongodb://localhost:27017` for local installation and `mongodb://mongo:27017` for docker

### Ansible configuration

- `ansible.inventory.folder`: Folder in which the deployment engine will store inventory information about deployments. It must be a folder writtable by the user which is running the application. By default it's `/tmp/ansible_inventories` although is **strongly** recommended to personalize this value if running locally. If running in docker this will be automatically configured to a safe location.
- `provision/ansible`: Folder containing the ansible scripts to deploy the different products. Some scripts are already provided in `provision/ansible` folder and that's the default value when running locally although it is **strongly** recommended too to provide a full path to this folder. When running in Docker this value will be automatically set.

## Usage 

Once installed, please read the [usage instructuions](usage.md)