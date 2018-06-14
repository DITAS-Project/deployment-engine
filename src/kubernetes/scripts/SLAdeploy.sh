#!/bin/bash
curl -o pod_deploy.yaml https://raw.githubusercontent.com/DITAS-Project/SLALite/master/pod_deploy.yaml
sed -i "s/name: slalite/$name: slalite$(date +%Y%m%d%H%M%S)/g" pod_deploy.yaml
export KUBECONFIG=$HOME/admin.conf
kubectl create -f pod_deploy.yaml

