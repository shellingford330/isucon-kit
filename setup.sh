#!/usr/bin/env bash

set -ue -o pipefail

read -p "Are you sure? [y/n]" -n 1 -r
echo    # (optional) move to a new line
if [[ ! $REPLY =~ ^[Yy]$ ]]
then
  printf "See you later!\n"
  exit 0
fi

# Pull application code & nginx, mysql config file
rsync --filter=":- .gitignore" -av isucon:/home/isucon/webapp/ ./webapp/
rsync -av isucon:/etc/nginx/ ./nginx/
rsync -av isucon:/etc/mysql/ ./mysql/
