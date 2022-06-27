#!/usr/bin/env bash

set -ue -o pipefail

read -p "Are you sure? [y/n]" -n 1 -r
echo    # (optional) move to a new line
if [[ ! $REPLY =~ ^[Yy]$ ]]
then
  printf "See you later!\n"
  exit 0
fi

# Pull application code
rsync -vr isucon:${APP_PATH} ./webapp/

# Pull Nginx configuration file
rsync -vr isucon:${NGINX_CONF_PATH} ./nginx/

# Pull MySQL configuration file
rsync -vr isucon:${MYSQL_CONF_PATH} ./mysql/
