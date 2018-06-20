#!/usr/bin/env bash
export KUBECONFIG=$HOME/admin.conf

curl -o cassandra-deployment.yaml https://raw.githubusercontent.com/DITAS-Project/deployment-engine/TUB/kubernetes-components/cassandra-deployment.yaml
sed -i "s/name: cassandra/name: cassandra$1/g" cassandra-deployment.yaml
sed -i "s/path: \/home\/cloudsigma\/blueprint/path: \/home\/cloudsigma\/blueprint$1/g" cassandra-deployment.yaml
kubectl create -f cassandra-deployment.yaml

curl -o cassandra-service.yaml https://raw.githubusercontent.com/DITAS-Project/deployment-engine/TUB/kubernetes-components/cassandra-service.yaml
sed -i "s/name: cassandra/name: cassandra$1/g" cassandra-service.yaml
kubectl create -f cassandra-service.yaml

curl -o mysql-deployment.yaml https://raw.githubusercontent.com/DITAS-Project/deployment-engine/TUB/kubernetes-components/mysql-deployment.yaml
sed -i "s/name: mysql/name: mysql$1/g" mysql-deployment.yaml
sed -i "s/path: \/home\/cloudsigma\/blueprint/path: \/home\/cloudsigma\/blueprint$1/g" mysql-deployment.yaml
kubectl create -f mysql-deployment.yaml

curl -o mysql-service.yaml https://raw.githubusercontent.com/DITAS-Project/deployment-engine/TUB/kubernetes-components/mysql-service.yaml
sed -i "s/name: mysql/name: mysql$1/g" mysql-service.yaml
kubectl create -f mysql-service.yaml

curl -o vdc-deployment.yaml https://raw.githubusercontent.com/DITAS-Project/deployment-engine/TUB/kubernetes-components/vdc-deployment.yaml
sed -i "s/name: vdc/name: vdc$1/g" vdc-deployment.yaml
sed -i "s/path: \/home\/cloudsigma\/blueprint/path: \/home\/cloudsigma\/blueprint$1/g" vdc-deployment.yaml
kubectl create -f vdc-deployment.yaml

curl -o vdc-service.yaml https://raw.githubusercontent.com/DITAS-Project/deployment-engine/TUB/kubernetes-components/vdc-service.yaml
sed -i "s/name: vdc/name: vdc$1/g" vdc-service.yaml
kubectl create -f vdc-service.yaml