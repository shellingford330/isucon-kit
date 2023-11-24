#!/usr/bin/env bash

set -ue -o pipefail

# Install analyzer for application
rsync -av ./Makefile isuconapp:~ 
ssh isuconapp 'make nginx/install-alp'

# Install analyzer for DB
rsync -av ./Makefile isucondb:~ 
ssh isucondb 'make mysql/install-pt-query-digest'
