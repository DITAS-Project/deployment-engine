import cloudsigma
import time
import os
import os.path

# ---# input data #--- #
node_names = ('node1', 'node2')
#node_names = ('sth', 'sth2')
dist_name = 'Ubuntu'
dist_version = '16.04 xenial'
ssh_user = 'ubuntu'
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
