#!/bin/bash
export KUBECONFIG=$HOME/admin.conf
kubectl create -f https://raw.githubusercontent.com/DITAS-Project/SLALite/master/pod_deploy.yaml

