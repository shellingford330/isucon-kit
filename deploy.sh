#!/usr/bin/env bash

set -ue

# Deploy source to remote 
rsync -av ./webapp/ isucon:/home/isucon/webapp/
rsync -av ./nginx/ isucon:/etc/nginx/
rsync -av ./mysql/ isucon:/etc/mysql/
rsync -av ./Makefile isucon:~

# Restart application server, middleware and so on ...
ssh isucon 'make restart'
