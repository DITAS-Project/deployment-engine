#!/usr/bin/env python
# This file is used to create a VM that serves as the deployment engine hardware,
# here the docker container with the app resides and to this address all deployment requests are directed.
import cloudsigma
import time
import os
import os.path
import sys

# ---# input data #--- #
# data from go
node_names = "deployment"
cpu = 2000
mem = 2048
numberOfMasters = 1
to_install = ('apt-get update', 'apt-get install -y python python-pip')
dist_name = 'Ubuntu'
dist_version = '16.04 DITAS' #change according to slack!
ssh_user = 'cloudsigma'
print "Checking if ssh rsa works"
pub_key = open(os.path.expanduser('~/.ssh/id_rsa.pub')).read()
print "Checking if ssh rsa works - yes"
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

# remove before creating new


def remove_old_vms(name):
       for serv_to_del in server.list():
           if str(serv_to_del.get('name')).startswith(name):
               serv_id_to_del = serv_to_del.get('uuid')
               if serv_to_del.get('status')!='stopped':
                   server.stop(serv_id_to_del)
                   _wait_until(serv_id_to_del, 'stopped')
               server.delete_with_disks(serv_id_to_del)    #it's removing vm with drive


print "Removing old nodes"
remove_old_vms(node_names)
time.sleep(2)
print "node: " + node_names + " removed"
print "all nodes removed"


def clone_lib_drive():
    lib_drive = [x['uuid'] for x in lib.list(query_params={'version': dist_version, 'distribution': dist_name})][0]
    drive.clone(lib_drive)


print "cloning drive..."
drives_to_clone = map(lambda x: clone_lib_drive(), range(1))

# some checks
busy_drives = 1
while busy_drives > 0:
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
freeDriveList = [s for s in drive.list() if s['status'] == 'unmounted']


for driv in freeDriveList: #drive.list()
    test_server = {
        'name': node_names, ##
        'cpu': cpu, ##
        'mem': mem * 1024 ** 2, ##
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
    print 'creating and starting ' + str(node_names)
    server.start(my_test_server['uuid'])
db_store = 'ansible-cloudsigma.db'

print 'waiting some time to let VM start ssh service...'
time.sleep(40)
print "VMs setup done!"
