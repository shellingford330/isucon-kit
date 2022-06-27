#!/usr/bin/env bash

set -ue -o pipefail

# Deploy source to remote 
rsync -vr ./webapp/ isucon:${APP_PATH}
rsync -vr ./nginx/ isucon:${NGINX_CONF_PATH}
rsync -vr ./mysql/ isucon:${MYSQL_CONF_PATH}
rsync -vr ./Makefile isucon:~

# Restart application server, middleware and so on ...
ssh isucon 'make restart'
