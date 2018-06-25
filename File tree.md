####This is a descriptive presentation of the files' structure of this project:
* jenkins
    * deploy
        - **deploy-staging.sh** - script for docker container depoyment
* **kubernetes-components** - yaml files for eaach component containing instructions to create pods/deployments/services/namespaces
    - cassandra-deployment.yaml
    - cassandra-service.yaml
    - ds4m.yaml
    - mysql-deployment.yaml
    - mysql-service.yaml
    - namespace.yaml
    - slalite.yaml
    - vdc-deployment.yaml
    - vdc-service.yaml
* src
    * api
        - **REST-Kubernetes_1.0.0_swagger.yaml** - old version of the api
        - **REST-Kubernetes_1.1.yaml** - last up-to-date version of the api
    * kubernetes
        * docs - old, legacy documentation of the first received version of the python VM creation script
        * roles
            * pre-deploy
                * defaults
                * meta
                * tasks
                    - **main.yml** - installation of pacakges such as docker, k8, https
                * vars
                - main.yml 
        * scripts
            - **ds4mdeploy.sh** - script that takes the corresponding kubernetes-components/x file and launches it in the k8 cluster with added index
            - **kube-flannel.yml** - configuration for flannel network for k8
            - **run.sh** - runs flannel setup
            - **SLAdeploy.sh** - as with ds4m
            - **TUBDummy.sh** - as with ds4m with added namespace - only for the first deployment
            - **TUBDummy_add.sh** - as with ds4m, without namespace - it already exists, for additional deployments
        - **ansible_deploy.yml** - ansible automatic deployment of the cluster and its components on the created VMs
        - **ansible_deploy_add.yml** - ansible automatic deployment of additional components in the same cluster - copying and creating a new version of each that should be copied
        - **create_vm.py** - python 2.7 script for creating virtual machines on Cloudsigma, parametrized for calls with GO REST API
        - **delete_vm.py** - python 2.7 script for removal of selected VMs on Cloudsigma
        - **inventory** - text file with IP of VMs to feed to Ansible to run the cluster setup there
        - **mysql.py** - script with python funcion to connect and update MySQL records with data obtained after VM creation such as IP
        - **create_vm_for_deployment.py** - python 2.7 script taken from the general VM creation and used to create a single, empty Ubuntu machine for storing an always ready version of deployment engine
    - **app.go** - go code for general methods creation
    - **main.go** - go main file
    - **model.go** - go code for specific methods creation
- **Dockerfile.artifact** - dockerfile for running the already build app with all dependencies such as MySQL connection or python scripts to call
- **Dockerfile.build** - dockerfile for building the app executable
- **Jenkinsfile** - text file with steps for continuous integration with Jenkins
- **README.md** - basic readme with all instructions necessary to run a full process
- **Useful commands for deployment engine** - additional commands to see the inside and have more control
- **File tree** - this file
    