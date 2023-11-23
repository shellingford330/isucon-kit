#!/usr/bin/env bash

set -ue -o pipefail

# Pull application code & nginx, mysql config file
rsync --filter=":- .gitignore" -av isucon:/home/isucon/webapp/ ./webapp/
rsync -av isucon:/etc/nginx/ ./nginx/
rsync -av isucon:/etc/mysql/ ./mysql/
