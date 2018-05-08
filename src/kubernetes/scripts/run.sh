#!/bin/bash
sudo cp /etc/kubernetes/admin.conf $HOME/admin.conf
sudo chown ${USER:=$(/usr/bin/id -run)}:$USER $HOME/admin.conf
export KUBECONFIG=$HOME/admin.conf
kubectl -n kube-system create -f https://raw.githubusercontent.com/coreos/flannel/master/Documentation/kube-flannel.yml
kubectl taint nodes --all node-role.kubernetes.io/master-
