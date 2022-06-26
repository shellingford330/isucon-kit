#!/usr/bin/env bash

set -ue -o pipefail

# Deploy source to remote 
rsync -vr ./webapp/ isucon@54.199.244.193:~/private_isu/webapp/golang/

# Restart application server, middleware and so on ...
make restart