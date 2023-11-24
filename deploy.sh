#!/usr/bin/env bash

set -ue

# Deploy source to remote 
rsync -av ./webapp/ isuconapp:/home/isucon/webapp/
rsync -av ./nginx/ isuconapp:/etc/nginx/
rsync -av ./mysql/ isucondb:/etc/mysql/
rsync -av ./Makefile isuconapp:~
rsync -av ./Makefile isucondb:~

# Restart application server and nginx
ssh isuconapp 'make app/restart'
ssh isuconapp 'make nginx/restart'
ssh isuconapp 'make nginx/rotate-log'

# Restart DB (mysql)
ssh isucondb 'make mysql/restart'
ssh isucondb 'make mysql/rotate-log'
