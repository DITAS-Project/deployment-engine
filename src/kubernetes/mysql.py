#!/usr/bin/env python
def update_ip_status(dict_ip, dict_status, cur, db):

    for key in set(dict_ip.keys()) & set(dict_status.keys()):
        cur.execute("""UPDATE nodesBlueprint SET nodesBlueprint.public_ip = %s, nodesBlueprint.status = %s WHERE nodesBlueprint.id = %s""",
                    (dict_ip[key], dict_status[key], key))
        db.commit()
