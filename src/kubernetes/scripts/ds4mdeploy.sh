#!/bin/bash
curl -o ds4m.yaml https://raw.githubusercontent.com/DITAS-Project/deployment-engine/master/kubernetes-components/ds4m.yaml
sed -i "s/name: ds4m/name: ds4m$(date +%Y%m%d%H%M%S)/g" ds4m.yaml
export KUBECONFIG=$HOME/admin.conf
kubectl create -f ds4m.yaml

