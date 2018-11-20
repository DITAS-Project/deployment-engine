# Usage guide

## Concepts

The REST interface and the internal business logic relies in a series of concepts and objects that can be found in `model/model.go` file:

### Input objects

- **Resource:** A resource is a Virtual Machine that needs to be created in a cloud provider. The information that needs to be passed is described in the `ResourceType` struct.
- **Infrastructure:** An infrastructure is a set of resources that need to be created in a particular cloud provider. The required properties are described in `InfrastructureType` struct.
- **Deployment:** A deployment represents an hybrid cloud or cloud-edge deployment formed by one or more infrastructures. For example, a deployment may compromise a set of VMs in AWS's EC2, another one in Google Cloud Platform and several machines already running in an edge location. The required properties are defined in `Deployment` struct.
- **Product:** A product is a software component that needs to be provisioned in one infrastructure. For example, `kubernetes` product will install Kubernetes in a particular infrastructure.

### Output object

- **Node:** A node is a Virtual Machine that has been created and it's running in a cloud provider. It's described in `NodeInfo` struct
- **Infrastructure Deployment:** Just as the input object, it represents a set of nodes that were successfully created in a cloud provider, separated by master and slaves. Each infrastructure has the unique identifier inside the deployment that was provided as input. It's described in `InfrastructureDeploymentInfo` struct.
- **Deployment information:** Represents an hybrid deployment. Just as its counterpart, it contains a list if infrastructures that were created and also it gets a unique identifier once the deployment process starts. It's described in `DeploymentInfo` struct.

## REST API

The Deployment Engine provides a default REST interface will listen by default in port 8080 unless configured otherwise (please, see the [installation instructions](installation.md) for the configuration options). The operations provided are:

- `POST /deployment`: Creates a new multi-infrastructure deployment with the resources provided in the request body. It returns the deployment information such as VM and Disk IDs and IPs assigned.
- `PUT /deployment/{depId}/{infraId}/{product}`: Provisions a product an infrastructure inside a deployment by providing the deployment and infrastructure identifiers as well as the desired product as path parameters.
- `DELETE /deployment/{depId}/{infraId}`: Removes an infrastructure in a deployment, clearing the resources such as VMs and disks that were allocated. If no more infrastructures remain in the deployment

## Example workflow

### Create a deployment

`POST /deployment`

#### Input

```json
{
   "name":"Deployment1",
   "description":"Test deployment in CloudSigma",
   "infrastructure":[
      {
         "name":"cloudsigma-deployment",
         "description":"Deployment in CloudSigma",
         "type":"cloud",
         "on-line":true,
         "provider":{
            "api_endpoint":"api url",
            "api_type":"cloudsigma",
            "keypair_id":"keypair_uuid"
         },
         "resources":[
            {
               "name":"master",
               "instance_type":"cpu=4000,000000,ram=4096",
               "type":"vm",
               "cpu":4000,
               "ram":4096,
               "disk":40960,
               "generate_ssh_keys":false,
               "ssh_keys_id":"uuid",
               "role":"master",
               "image_id":"a2a67f55-c775-4871-808d-53136e31d2f0",
               "drives":[
                  {
                     "name":"data",
                     "type":"SDD",
                     "size":10240
                  }
               ]
            },
            {
               "name":"slave",
               "instance_type":"cpu=2000,000000,ram=4096",
               "type":"vm",
               "cpu":2000,
               "ram":4096,
               "disk":40960,
               "generate_ssh_keys":false,
               "ssh_keys_id":"uuid",
               "role":"slave",
               "image_id":"a2a67f55-c775-4871-808d-53136e31d2f0"
            }
         ]
      }
   ]
}
```

#### Output

```json
{
   "id":"84e259c1-676f-4b72-91c4-6cce33315593",
   "infrastructures":[
      {
         "id":"cloudsigma-deployment",
         "type":"cloudsigma",
         "provider":{
            "api_endpoint":"api url",
            "api_type":"cloudsigma",
            "keypair_id":"keypair_uuid"
         },
         "slaves":[
            {
               "hostname":"cloudsigma-deployment-slave",
               "role":"slave",
               "ip":"85.204.96.36",
               "username":"cloudsigma",
               "uuid":"d32bc930-8288-4625-b9e4-57f1dd5612c1",
               "drive_uuid":"5f8ce652-9c91-44ce-9149-eacf5e9fe270",
               "data_drives":[

               ]
            }
         ],
         "master":{
            "hostname":"cloudsigma-deployment-master",
            "role":"master",
            "ip":"85.204.97.196",
            "username":"cloudsigma",
            "uuid":"a78b2781-2251-45f0-bc38-e2798e5880b1",
            "drive_uuid":"d1bc522b-e51c-422b-8e1a-f944542da96b",
            "data_drives":[
               {
                  "uuid":"4a5d85a4-279c-459a-88b4-fa43f042ca9b",
                  "name":"data-cloudsigma-deployment-master-data"
               }
            ]
         },
         "status":"running",
         "products":null
      }
   ],
   "status":"running"
}
```

### Deploy kubernetes

`PUT /deployment/84e259c1-676f-4b72-91c4-6cce33315593/cloudsigma-deployment/kubernetes`

#### Input

#### Output

```json
{
   "id":"84e259c1-676f-4b72-91c4-6cce33315593",
   "infrastructures":[
      {
         "id":"cloudsigma-deployment",
         "type":"cloudsigma",
         "provider":{
            "api_endpoint":"api url",
            "api_type":"cloudsigma",
            "keypair_id":"keypair_uuid"
         },
         "slaves":[
            {
               "hostname":"cloudsigma-deployment-slave",
               "role":"slave",
               "ip":"85.204.96.36",
               "username":"cloudsigma",
               "uuid":"d32bc930-8288-4625-b9e4-57f1dd5612c1",
               "drive_uuid":"5f8ce652-9c91-44ce-9149-eacf5e9fe270",
               "data_drives":[

               ]
            }
         ],
         "master":{
            "hostname":"cloudsigma-deployment-master",
            "role":"master",
            "ip":"85.204.97.196",
            "username":"cloudsigma",
            "uuid":"a78b2781-2251-45f0-bc38-e2798e5880b1",
            "drive_uuid":"d1bc522b-e51c-422b-8e1a-f944542da96b",
            "data_drives":[
               {
                  "uuid":"4a5d85a4-279c-459a-88b4-fa43f042ca9b",
                  "name":"data-cloudsigma-deployment-master-data"
               }
            ]
         },
         "status":"running",
         "products":[
            "kubernetes"
         ]
      }
   ],
   "status":"running"
}
```

### Delete ifrastructure

`DELETE /deployment/84e259c1-676f-4b72-91c4-6cce33315593/cloudsigma-deployment`

#### Input

#### Output

```json
{
    "id": "",
    "infrastructures": null,
    "status": ""
}
```