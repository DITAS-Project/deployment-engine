# Developer's guide

## Extension

This project can be extended on different project domains by creating a folder which will host the files which are only relevant for the extension. For example, if the project is named *superproject* then create a folder named `superproject` in the root to include all files which are project-specific. That way, contributions upstream are simpler since everything in this folder must not be commited back to the generic project while changes in the rest of the folders might be contributed back if they are useful across different projects.

For an example of an extension, please have a look at the [Ditas Project's Deployment Engine](https://github.com/DITAS-Project/deployment-engine)

## Concepts

The code in this project tries to be as generic and extensible as possible without the need to touch too much the core parts. That extensibility allows it to be adapted to different needs across projects. To do so, it uses a series of concepts as shown below:

### Deployer

A deployer is a component which is able to create and delete resources in a particular infrastructure type with the information included in the `InfrastructureType` struct. It must comply with the `Deployer` interface described in `model/model.go` file. A deployer for CloudSigma is included in `infrastructure/cloudsigma` folder. To develop new deployers:

- If they are generic enough (for example, a deployer for AWS, Google Cloud, or any other cloud provider) put it in a subfolder in `infrastructure` and give it a package name.
- If it's a project-specific deployer, put it in the project-specific folder instead.
- Once done, modify the `findProvider` function in `infrastructure/deploymentcontroller.go` file taking into account the new deployer that you just created.

### Provisioner

A provisioner is a component which, given an infrastructure, is able to deploy different products over it. An Ansible provisioner which is able to deploy kubernetes is provided in the `provison/ansible` folder. New provisioners must comply with the `Provisioner` interface defined in `model/model.go` file. To develop new provisioners:

- If it's based on Ansible and the product is generic enough (i.e. Jenkins, Mesos, Marathon, etc), modify the ansible provisioner to include the new product. Take into account that Go allows to define methods of a struct in different files, so feel free to create a new file for your product. Also, for consistency, include the ansible scripts into the ansible folder in a subfolder with the product name.
- If it's not based in Ansible then create a new subfolder in `provision` with the name of the deployer (i. e. chef or puppet) and implement the `Provisioner` interface. Then pass the provisioner to the `ProvisionerController` in `main.go` when creating it.
- Whatever the provisioner, be it based on Ansible or not, if the product is project-specific, please put its files inside the project-specific folder and then use the default Ansible or any other provider methods as helpers (for example, to create the inventory and initialize the ssh known_hosts) but do the rest of the deployment as well as put the rest of the scripts in another subfolder and package in the project-specific folder

### Frontend

Most projects will need a frontend API to interact with the deployers and provisioners. As a helper, a default REST API is implemented in `restfrontend/restfrontend.go` with the operations described in the [usage guide](usage.md). To extend it:

- If the operations added are generic and based on the current datamodel, or a generic extension, then modify the default `restfrontend.go` file to include your changes.
- If the operation or the datamodel it uses is project-specific then create a new frontend implementation in the project-specific folder and use the default frontend as a helper.