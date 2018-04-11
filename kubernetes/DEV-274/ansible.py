#!/usr/bin/env python
### DEPRECIATED
import os
import sys
import subprocess
def runAnsible():
#print "Running ansible playbook by: ANSIBLE_NOCOWS=1 ansible-playbook ansible_deploy.yml -i inventory"
    script_dir = os.path.dirname(__file__)
    rel_path = 'ansible_deploy.yml'
    rel_path2 = 'inventory'
    abs_file_path = os.path.join(script_dir, rel_path)
    abs_file_path2 = os.path.join(script_dir, rel_path2)
    print("Running ANSIBLE_NOCOWS=1 ansible-playbook " + abs_file_path + " -i " + abs_file_path2)
    #print subprocess.Popen("ANSIBLE_NOCOWS=1 ansible-playbook ansible_deploy.yml -i inventory", shell=True, stdout=subprocess.PIPE).stdout.read()
    print subprocess.Popen("ANSIBLE_NOCOWS=1 ansible-playbook " + abs_file_path + " -i " + abs_file_path2, shell=True, stdout=subprocess.PIPE).stdout.read()
    print "All done"
#runAnsible()
