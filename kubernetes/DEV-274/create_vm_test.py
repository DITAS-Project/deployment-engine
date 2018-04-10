#!/usr/bin/env python
import cloudsigma
import time
import os
import os.path
import sys

# ---# input data #--- #
## data from go
node_names = []
cpu = []
mem = []
print "Node names: "
for i in range (1, len(sys.argv), 3):
    print sys.argv[i]
    node_names.append(sys.argv[i])
print "RAM sizes: "
for i in range (2, len(sys.argv), 3):
    print sys.argv[i]
    mem.append(int(sys.argv[i]))
print "CPU sizes: "
for i in range (3, len(sys.argv), 3):
    print sys.argv[i]
    cpu.append(int(sys.argv[i]))
##
#node_names = ('node1', 'node2') ##
to_install = ('apt-get update', 'apt-get install -y python python-pip', 'reboot')
dist_name = 'Ubuntu'
dist_version = '16.04'
ssh_user = 'cloudsigma'
pub_key = open(os.path.expanduser('~/.ssh/id_rsa.pub')).read()
# ---# end of input data #--- #

drive = cloudsigma.resource.Drive()
server = cloudsigma.resource.Server()
lib = cloudsigma.resource.LibDrive()

def _wait_until(uuid, status_required, timeout=60):
       status = server.get(uuid=uuid)['status']
       while status != status_required and timeout > 0:
           time.sleep(1)
           timeout -= 1
           status = server.get(uuid=uuid)['status']
#remove before creating new
def remove_old_vms(name):
       for serv_to_del in server.list():
           if str(serv_to_del.get('name')).startswith(name):
               serv_id_to_del = serv_to_del.get('uuid')
               if serv_to_del.get('status')!='stopped':
                   server.stop(serv_id_to_del)
                   _wait_until(serv_id_to_del, 'stopped')
                   #_wait_until(serv_id_to_del, 'stopped')   #just a method to check vm status and wait if its not 'stopped'
               server.delete_with_disks(serv_id_to_del)    #it's removing vm with drive

print "removing old nodes"
for node in node_names:
    remove_old_vms(node)
    time.sleep(2)
    print "node: " + node + " removed"
print "all nodes removed"

def clone_lib_drive():
    lib_drive = [x['uuid'] for x in lib.list(query_params={'version': dist_version, 'distribution': dist_name})][0]
    drive.clone(lib_drive)


print "cloning " + str(len(node_names)) + " drives..."
drives_to_clone = map(lambda x: clone_lib_drive(), range(len(node_names)))

# some checks
busy_drives = 1
while busy_drives > 0:
    busy_drives = len(node_names)
    time.sleep(3)
    print '---checking drives---'
    for i in cloudsigma.resource.Drive().list():

        if i['status'] == 'unmounted':
            print 'drive ready'
            busy_drives -= 1
        if i['status'] != 'unmounted':
            print "drive is in " + str(i['status']) + " status - rechecking again"

else:
    busy_drives = 0
    print '---finished drive check, OK---'


def check_run():
    check = 0
    while check != 0:
        check = 0
        for run_check in cloudsigma.resource.Server().list():
            if run_check['status'] == 'running':
                pass
            else:
                check += 1
    return 'servers are online'

# nodes creation
for name, driv, memory, cpus in zip(node_names, drive.list(), mem, cpu):
    test_server = {
        'name': name, ##
        'cpu': cpus, ##
        'mem': memory * 1024 ** 2, ##
        'vnc_password': 'test_server'
    }
    my_test_server = server.create(test_server)

    my_test_server['drives'] = [{
        'boot_order': 1,
        'dev_channel': '0:0',
        'device': 'virtio',
        'drive': driv['uuid']
    }]
    my_test_server['nics'] = [{
        'ip_v4_conf': {
            'conf': 'dhcp',
            'ip': None
        },
        'model': 'virtio',
        'vlan': None
    }]
    my_test_server['meta'] = {'ssh_public_key': str(pub_key)}
    server.update(my_test_server['uuid'], my_test_server)
    print 'creating and starting ' + str(name)
    server.start(my_test_server['uuid'])
db_store = 'ansible-cloudsigma.db'

print 'waiting some time to let VM start ssh service...'
time.sleep(80)


def refresh_db():
    ansible_db = {}
    running_uuid = []

    get_servers = cloudsigma.resource.Server()
    server_list = get_servers.list()

    script_dir = os.path.dirname(__file__)
    rel_path = 'inventory'
    abs_file_path = os.path.join(script_dir, rel_path)
    with open(abs_file_path, 'w') as cleanfile:
        cleanfile.write('')
    count = 0
    for vm in sorted(server_list):

        ipv4 = vm['runtime']['nics'][0]['ip_v4']['uuid']
        vm_name = vm['name']
        print "generating hosts file on " + str(vm_name)
        for host_server in sorted(server_list):
            hosts_ipv4 = host_server['runtime']['nics'][0]['ip_v4']['uuid']
            hosts_name = host_server['name']
            os.system('ssh -o "StrictHostKeyChecking no" ' + str(ssh_user) + '@' + str(
                ipv4) + " '" + 'sudo echo ' + '"' + str(hosts_ipv4) + ' ' + str(
                hosts_name) + '"' + ' | sudo tee -a /etc/hosts' + "' > /dev/null 2>&1")
        for install in to_install:
            print "executing " + '"' + str(install) + '"' + " on " + str(vm_name) + "..."
            os.system('ssh -o "StrictHostKeyChecking no" ' + ssh_user+'@' + str(
                ipv4) + " sudo " + install + " > /dev/null 2>&1")

        if count == 1:
            stream = '[slaves]\n'
        else:
            stream = ''
        with open(abs_file_path, 'a+') as f:
            f.write(str(("[master]\n" if count == 0 else str(stream))) + str(
                vm_name) + ' ansible_ssh_host=' + str(
                ipv4) + ' ansible_ssh_user=' + str(ssh_user) + '\n')
        count += 1
        running_uuid.append(vm['uuid'])
        ansible_db[vm_name] = {'ansible_ssh_host': ipv4}

refresh_db()
print "Done! Check the inventory file"

#run ansible playbook
#import ansible
#ansible.runAnsible()
