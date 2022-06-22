#!/bin/sh

rm /var/log/mysql/mysql-slow.log
# ファイルが更新されていることをMySQLに伝える
mysqladmin flush-logs