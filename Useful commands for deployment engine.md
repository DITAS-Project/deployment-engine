-------------------CURL create/delete deployment - run it in any command line

curl -H "Content-Type: application/json" -d '{"name":"test", "description":"AddedDep", "on-line":"starting", "type":"x", "api_endpoint":"xx", "api_type":"xxx", "keypair_id":"xxxx", "resources": [{"name": "sth1", "role": "none", "ram":2048, "cpus":2000, "type": "none", "disc": "none", "generate_ssh_keys": "none", "ssh_keys_id": "none", "baseimage": "none", "arch": "none", "os": "none"}, {"name": "sth2", "role": "none", "ram":2048, "cpus":2000, "type": "none", "disc": "none", "generate_ssh_keys": "none", "ssh_keys_id": "none", "baseimage": "none", "arch": "none", "os": "none"}]}' 31.171.247.156:50012/dep


curl -X DELETE 31.171.247.156:50012/dep/test
curl 31.171.247.156:50012/dep/test
-------------------Login to deployment engine VM

ssh cloudsigma@31.171.247.156

-------------------Download and run docker artifact

docker pull ditas/deployment-engine:latest

docker stop --time 20 deployment-engine

docker rm --force deployment-engine

docker run --name=mysql -p 50013:3306 -e MYSQL_ROOT_PASSWORD=root -e MYSQL_DATABASE=k8sql -d mysql:5.7.22

docker run -p 50012:8080 -d --name deployment-engine --link mysql:mysql ditas/deployment-engine:latest

-------------------Get into docker container, into master of the network and check kubernetes

docker exec -it deployment-engine bash

ssh cloudsigma@masterIP

export KUBECONFIG=$HOME/admin.conf

kubectl get nodes -o wide

//done automatically in branch SLA
kubectl create -f https://raw.githubusercontent.com/DITAS-Project/SLALite/master/pod_deploy.yaml

kubectl describe pod slalite

//get into kubelet
kubectl exec -it slalite -- /bin/sh

kubectl delete svc slalite | kubectl delete pod slalite
-----------------------------


