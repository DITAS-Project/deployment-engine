#!/bin/bash
curl -o slalite.yaml https://raw.githubusercontent.com/DITAS-Project/deployment-engine/master/kubernetes-components/slalite.yaml
sed -i "s/name: slalite/name: slalite$(date +%Y%m%d%H%M%S)/g" slalite.yaml
export KUBECONFIG=$HOME/admin.conf
kubectl create -f slalite.yaml

