#!/usr/bin/env bash

set -ue -o pipefail

# Deploy source to remote 
rsync -av ./webapp/ isucon:${APP_PATH}
rsync -av ./nginx/ isucon:${NGINX_CONF_PATH}
rsync -av ./mysql/ isucon:${MYSQL_CONF_PATH}
rsync -av ./Makefile isucon:~

# Restart application server, middleware and so on ...
ssh isucon 'make restart'
