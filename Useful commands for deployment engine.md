### CURL create/delete deployment - run it in any command line
ADD

`curl -H "Content-Type: application/json" -d '{"name":"test", "description":"AddedDep", "on-line":"starting", "type":"x", "api_endpoint":"xx", "api_type":"xxx", "keypair_id":"xxxx", "resources": [{"name": "sth1", "role": "none", "ram":2048, "cpus":2000, "type": "none", "disc": "none", "generate_ssh_keys": "none", "ssh_keys_id": "none", "baseimage": "none", "arch": "none", "os": "none"}, {"name": "sth2", "role": "none", "ram":2048, "cpus":2000, "type": "none", "disc": "none", "generate_ssh_keys": "none", "ssh_keys_id": "none", "baseimage": "none", "arch": "none", "os": "none"}]}' 31.171.247.156:50012/dep`

VIEW ONE 

`curl 31.171.247.156:50012/dep/test`

REMOVE

`curl -X DELETE 31.171.247.156:50012/dep/test`
### Log into deployment engine VM

`ssh cloudsigma@31.171.247.156`

### Download and run docker artifact

`docker pull ditas/deployment-engine:latest && docker stop --time 20 deployment-engine && docker rm --force deployment-engine`

Only if non-existent (check with docker ps):
`docker run --name=mysql -p 50013:3306 -e MYSQL_ROOT_PASSWORD=root -e MYSQL_DATABASE=k8sql -d mysql:5.7.22`

Run
`docker run -p 50012:8080 -d --name deployment-engine --link mysql:mysql ditas/deployment-engine:latest`

### Get into docker container, into master of the network and check kubernetes

`docker exec -it deployment-engine bash`

`ssh cloudsigma@masterIP` where masterIP is whatever IP belonging to the first machine form the blueprint

`export KUBECONFIG=$HOME/admin.conf`

`kubectl get nodes -o wide` optionally to view nodes

`kubectl get pods`

`kubectl describe pod slaliteXXX` where XXX is a number to view details of the pod.

`kubectl exec -it slaliteXXX -- /bin/sh` to get into the pod

`kubectl delete svc slalite && kubectl delete pod slalite` to remove the pod and the service



