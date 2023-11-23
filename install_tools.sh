#!/usr/bin/env bash

set -ue -o pipefail

# Install analyzer log tool
rsync -av ./Makefile isucon:~ 
ssh isucon 'make install'
