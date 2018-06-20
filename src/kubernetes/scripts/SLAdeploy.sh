#!/bin/bash
curl -o slalite.yaml https://raw.githubusercontent.com/DITAS-Project/deployment-engine/master/kubernetes-components/slalite.yaml
sed -i "s/name: slalite/name: slalite$1/g" slalite.yaml
sed -i "s/path: \/home\/cloudsigma\/blueprint/path: \/home\/cloudsigma\/blueprint$1/g" slalite.yaml
export KUBECONFIG=$HOME/admin.conf
kubectl create -f slalite.yaml

