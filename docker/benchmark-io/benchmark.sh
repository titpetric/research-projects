#!/bin/bash
wrk -t8 -c1000 -d50s -s post.lua http://$1:8080/api/sign