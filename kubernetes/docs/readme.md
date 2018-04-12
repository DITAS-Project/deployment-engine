Requirements:
* no drives in the account(script should be modified in other case)
* manual input OS and packages_to_install into python file
* create_vm.py tested with ubuntu,but should work with any other OS(installing packets will pass)

Usage:
python2.7 create_vm.py
ANSIBLE_NOCOWS=1 ansible-playbook ansible_deploy.yml -i inventory
