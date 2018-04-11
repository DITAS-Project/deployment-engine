#!/usr/bin/env python
import MySQLdb
import mysql
db = MySQLdb.connect(host="localhost",    # your host, usually localhost
                     user="root",         # your username
                     passwd="root",  # your password
                     db="k8sql")        # name of the data base
cur = db.cursor()

dict_ip = {}
dict_status = {}
dict_ip['sth'] = '6.1.1.1'
dict_status['sth'] = 'running'
dict_ip['sth2'] = '6.2.2.2'
dict_status['sth2'] = 'running'
mysql.update_ip_status(dict_ip, dict_status, cur, db)
db.close()
print 'end'