**Pull kubernetes from GIT and deploy it with ansible**

Steps:

# git clone ssh://your_user@91.92.71.107:2522/srv/git/repos/kubernetes.git

Structure:

* ./kubernetes
	* README.MD
	* DEV-274
		* **docs** - documentation for deploying kubernetes
		* **roles**	- role to install required packages
		* **scripts** - dir with 2 files for ansible role
		* **ansible_deploy.yml** - playbook to deploy kubernetes cluster
		* **create_vm.py** - script to create vm(tested on ZRH) and inventory file

First, we need to create VMs:

# **python2.7 create_vm.py**
(by default it creates 3 vm with Ubuntu16.04 xenial and hostnames node1, node2, node3)

To deploy cluster you need to run playbook(generated inventory file required):

# **ansible-playbook ansible_deploy.yml -i inventory**

After that we can connect to master node and check the cluster status:

# **kubectl cluster-info**
# **kubectl get nodes**
# **kubectl get pods --all-namespaces**
