#!/bin/bash
export KUBECONFIG=$HOME/admin.conf
kubectl create -f $1-$2.yaml

