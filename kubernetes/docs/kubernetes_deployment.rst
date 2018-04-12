**Kubernetes deployment**

Content:


1. Installing base packets (on every node)                         
2. Deploying and configuring kubernetes (under root on master node) 
3. Join slave nodes (under root on all slaves)                      


**1. Installing base packets(on every node)**

# **apt-get update && apt-get install -y apt-transport-https**
Add repo key to use HTTPS:
# **curl -s https://packages.cloud.google.com/apt/doc/apt-key.gpg | apt-key add -**


Add kubernetes repo:
# **cat <<EOF >/etc/apt/sources.list.d/kubernetes.list**
**deb http://apt.kubernetes.io/ kubernetes-xenial main**
**EOF**

Install docker-compose (1.9.0 version used)
I used next params:

url: https://github.com/docker/compose/releases/download/1.9.0/docker-compose-Linux-x86_64

dest: /usr/local/bin/docker-compose, mode: "a+x"

Install list of packages(current kubeadm version is 1.6.1):
# **apt-get install -y kubelet kubeadm kubectl kubernetes-cni docker.io**

**2. Deploying and configuring kubernetes (under root on master node)**
# **kubeadm init --pod-network-cidr 10.244.0.0/16 | grep "kubeadm join —token"**
(or you can use it without **grep** to get all output info including instructions of getting access to kubernetes master). We need to save somewhere the output to use it later.

**under regular user on master node:**
# **sudo cp /etc/kubernetes/admin.conf $HOME/admin.conf**
# **sudo chown ${USER:=$(/usr/bin/id -run)}:$USER $HOME/admin.conf**

every time we need to use kubernetes commands we need to use that var(it should not be empty):
# **export KUBECONFIG=$HOME/admin.conf**

now we can use regular kubernetes commands like:
# kubectl get nodes
# kubectl get pods --all-namespaces
# kubectl get pods 
OR to exclude every time use command  export KUBECONFIG=$HOME/admin.conf you can do:
# **cp $HOME/admin.conf $HOME/.kube/config**
# **sudo chown ${USER:=$(/usr/bin/id -run)}:$USER $HOME/.kube/config**
# **sudo chmod 766 $HOME/.kube/config (just in case, 600 not worked after master reboot)**

Then we can configure flannel networks with two config files:

# **kubectl create -f https://raw.githubusercontent.com/coreos/flannel/master/Documentation/kube-flannel-rbac.yml**

!!! next config file uses the older version which is not working correctly:
https://raw.githubusercontent.com/coreos/flannel/master/Documentation/kube-flannel.yml
So we need to download that file and change the lines like:
**image: quay.io/coreos/flannel:v0.7.0-amd64**
to
**image: quay.io/coreos/flannel:v0.7.1-amd64**
(there is two of such lines №58 and №69 so we need to change both)
and after that we can run:
# **kubectl create -f** (the path to our saved and edited file) **.yml**

By default, your cluster will not schedule pods on the master for security reasons. If you want to be able to schedule pods on the master, e.g. a single-machine Kubernetes cluster for development, run:
# **kubectl taint nodes --all node-role.kubernetes.io/master-**
(taken from https://kubernetes.io/docs/getting-started-guides/kubeadm/ so you can read more about)

Now the master should be ready and we can check it with next commands:

kubectl get nodes 

kubectl get pods --all-namespaces 

kubectl cluster-info

kubectl cluster-info dump


**3. Join slave nodes (under root on all slaves)**
We need to use the output of kubeadm init (page №2), just run the output on all slave nodes, it would be like:
# **kubeadm join --token 93e857.936936279a915613 178.22.70.66:6443**




**Deployment finished.**
