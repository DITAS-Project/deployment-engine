#!/bin/bash
curl -o ds4m.yaml https://raw.githubusercontent.com/DITAS-Project/deployment-engine/master/kubernetes-components/ds4m.yaml
sed -i "s/name: ds4m/name: ds4m$1/g" ds4m.yaml
sed -i "s/path: \/home\/cloudsigma\/blueprint/path: \/home\/cloudsigma\/blueprint$1/g" ds4m.yaml

export KUBECONFIG=$HOME/admin.conf
kubectl create -f ds4m.yaml

