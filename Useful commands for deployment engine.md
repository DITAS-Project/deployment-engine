# CURL create/delete deployment - run it in any command line

## ADD A DEPLOYMENT

`curl -H "Content-Type: application/json" -d '{"name":"test", "description":"AddedDep", "on-line":"starting", "type":"x", "api_endpoint":"xx", "api_type":"xxx", "keypair_id":"xxxx", "resources": [{"name": "sth1", "role": "none", "ram":4096, "cpus":2000, "type": "none", "disc": "none", "generate_ssh_keys": "none", "ssh_keys_id": "none", "baseimage": "none", "arch": "none", "os": "none"}, {"name": "sth2", "role": "none", "ram":4096, "cpus":2000, "type": "none", "disc": "none", "generate_ssh_keys": "none", "ssh_keys_id": "none", "baseimage": "none", "arch": "none", "os": "none"}]}' 31.171.247.156:50012/deps`

## VIEW SELECTED DEPLOYMENT

`curl 31.171.247.156:50012/deps/test`

## REMOVE SELECTED DEPLOYMENT

`curl -X DELETE 31.171.247.156:50012/deps/test?deleteDeployment=true`

### Log into deployment engine VM

`ssh cloudsigma@31.171.247.156`

### Download and run docker artifact

This is done inside of the deployment VM to pull the newest version of the engine. 

First stop the running Docker Compose by `ctrl+c` if it's in blocking mode or by `docker-compose down` at the `$HOME` folder. Then execute:

`docker pull ditas/deployment-engine:latest && docker-compose up`

### Get into docker container - engine, into master of the network and check kubernetes

`docker exec -it cloudsigma_deployment-engine_1 bash`

`ssh cloudsigma@masterIP` where masterIP is whatever IP belonging to the first machine from the blueprint - it is now the kubernetes master

`kubectl get nodes -o wide` optionally to view nodes

`kubectl get pods --all-namespaces` to view all pods, regardless of their definition

`kubectl describe pod slaliteXXX`, where XXX is a number, to view details of the pod. If the pod has a namespace add `--namespace=YYY`

`kubectl exec -it slaliteXXX -- /bin/sh` to get into the pod's shell

`kubectl delete svc slaliteXXX && kubectl delete pod slaliteXXX` to remove the pod and the service. If the pod is deployed with a deployment instead of removing pods and services run `kubectl delete deployment slaliteXXX` to remove everything at once. Remark about namespaces is still valid.



