#!/usr/bin/env python
def update_ip_status(dict_ip, dict_status, cur, db):

    for key in set(dict_ip.keys()) & set(dict_status.keys()):
        cur.execute("""UPDATE nodes SET nodes.public_ip = %s, nodes.status = %s WHERE nodes.id = %s""", (dict_ip[key], dict_status[key], key))
        db.commit()
