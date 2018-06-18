#!/usr/bin/env bash
export KUBECONFIG=$HOME/admin.conf

curl -o cassandra-deployment.yaml https://raw.githubusercontent.com/DITAS-Project/tub-dummy-example/k8s/cassandra-deployment.yaml
sed -i "s/name: cassandra/name: cassandra$(date +%Y%m%d%H%M%S)/g" cassandra-deployment.yaml
kubectl create -f cassandra-deployment.yaml

curl -o cassandra-service.yaml https://raw.githubusercontent.com/DITAS-Project/tub-dummy-example/k8s/cassandra-service.yaml
sed -i "s/name: cassandra/name: cassandra$(date +%Y%m%d%H%M%S)/g" cassandra-service.yaml
kubectl create -f cassandra-service.yaml

curl -o mysql-deployment.yaml https://raw.githubusercontent.com/DITAS-Project/tub-dummy-example/k8s/mysql-deployment.yaml
sed -i "s/name: mysql/name: mysql$(date +%Y%m%d%H%M%S)/g" mysql-deployment.yaml
kubectl create -f mysql-deployment.yaml

curl -o mysql-service.yaml https://raw.githubusercontent.com/DITAS-Project/tub-dummy-example/k8s/mysql-service.yaml
sed -i "s/name: mysql/name: mysql$(date +%Y%m%d%H%M%S)/g" mysql-service.yaml
kubectl create -f mysql-service.yaml

curl -o vdc-deployment.yaml https://raw.githubusercontent.com/DITAS-Project/tub-dummy-example/k8s/vdc-deployment.yaml
sed -i "s/name: vdc/name: vdc$(date +%Y%m%d%H%M%S)/g" vdc-deployment.yaml
sed -i "s/tub_example_vdc_dummy:v01/tub_example_vdc_dummy/g" vdc-deployment.yaml
kubectl create -f vdc-deployment.yaml

curl -o vdc-service.yaml https://raw.githubusercontent.com/DITAS-Project/tub-dummy-example/k8s/vdc-service.yaml
sed -i "s/name: vdc/name: vdc$(date +%Y%m%d%H%M%S)/g" vdc-service.yaml
kubectl create -f vdc-service.yaml