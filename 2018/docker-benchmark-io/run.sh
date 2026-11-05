#!/bin/bash
docker rm -f dco
rm dco.sqlite3 -f
docker run --name dco -d --ulimit nofile=100000:100000 --restart=always -v $(pwd):/data -p 8080:8080 dco