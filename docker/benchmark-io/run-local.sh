#!/bin/bash
if [ ! -f "dco" ]; then
	./run.sh
	docker exec -it dco cp /dco/dco /data
fi
docker rm -f dco
rm dco.sqlite3 -f
screen -dmS dco bash -c "ulimit -n 100000 ; ./dco"