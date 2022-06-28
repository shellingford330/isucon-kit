SHELL:=/bin/bash -e -o pipefail
COLOR_GREEN:=\u001b[32m
COLOR_DEFAULT:=\u001b[30m

MYSQL_SLOW_LOG_PATH:=/var/log/mysql/mysql-slow.log

default: help

## This help screen
help:
	@printf "Available targets:\n\n"
	@awk '/^[a-zA-Z\-\_0-9%:\\]+/ { \
		helpMessage = match(lastLine, /^## (.*)/); \
		if (helpMessage) { \
			helpCommand = $$1; \
			helpMessage = substr(lastLine, RSTART + 3, RLENGTH); \
			gsub("\\\\", "", helpCommand); \
			gsub(":+$$", "", helpCommand); \
			printf "  \x1b[32;01m%-35s\x1b[0m %s\n", helpCommand, helpMessage; \
		} \
	} \
	{ lastLine = $$0 }' $(MAKEFILE_LIST) | sort -u
	@printf "\n"

## Restart server
restart: app/restart mysql/restart nginx/restart mysql/rotate_log nginx/rotate_log
	@printf "${COLOR_GREEN}Success!${COLOR_DEFAULT}\n"

## [App] Restart server
app/restart:
	systemctl restart isu-go

## [MySQL] Restart server
mysql/restart:
	systemctl restart mysql

## [MySQL] Rotate log file
mysql/rotate_log:
	rm ${MYSQL_SLOW_LOG_PATH}
	# ファイルが更新されていることをMySQLに伝える
	mysqladmin flush-logs

## [MySQL] Install pt-query-digest
mysql/install_pt_query_digest:
	apt-get update
	apt-get install percona-toolkit

## [MySQL] Run pt-query-digest
mysql/pt_query_digest:
	pt-query-digest ${MYSQL_SLOW_LOG_PATH} | head -n 30

## [MySQL] Run mysqldumpslow
mysql/mysqldumpslow:
	mysqldumpslow ${MYSQL_SLOW_LOG_PATH}

## [Nginx] Restart server
nginx/restart:
	nginx -t
	systemctl reload nginx

## [Nginx] Rotate log file
nginx/rotate_log:
	# 実行時点の日時を付与したファイル名にローテートする
	mv /var/log/nginx/access.log /var/log/nginx/access.log.$(date +%Y%m%d-%H%M%S)
	# nginxにログファイルを開き直すシグナルを送信する
	nginx -s reopen
