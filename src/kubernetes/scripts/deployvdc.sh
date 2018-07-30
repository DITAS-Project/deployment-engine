#!/bin/bash
export KUBECONFIG=$HOME/admin.conf
kubectl create -f vdc$1.yaml

