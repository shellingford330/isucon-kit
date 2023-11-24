#!/usr/bin/env bash

set -ue -o pipefail

# Pull application code & nginx, mysql config file
rsync --filter=":- .gitignore" -av isuconapp:/home/isucon/webapp/ ./webapp/
rsync -av isuconapp:/etc/nginx/ ./nginx/
rsync -av isucondb:/etc/mysql/ ./mysql/
