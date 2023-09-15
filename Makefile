SHELL:=/bin/bash -e -o pipefail
COLOR_GREEN:=\u001b[32m
COLOR_DEFAULT:=\u001b[30m

MYSQL_SLOW_LOG_PATH:=/var/log/mysql/mysql-slow.log
NGINX_ACCESS_LOG_PATH:=/var/log/nginx/access.log

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
restart: app/restart mysql/restart nginx/restart mysql/rotate-log nginx/rotate-log
	@printf "${COLOR_GREEN}Success!${COLOR_DEFAULT}\n"

# Install tool
install: mysql/install-pt-query-digest nginx/install-alp
	@printf "${COLOR_GREEN}Success!${COLOR_DEFAULT}\n"

# Analyze
analyze: mysql/pt-query-digest mysql/mysqldumpslow nginx/alp
	@printf "${COLOR_GREEN}Success!${COLOR_DEFAULT}\n"

## [App] Restart server
app/restart:
	systemctl restart isuports.service

## [App] Build
app/build:
	cd webapp/go && make build

## [MySQL] Restart server
mysql/restart:
	systemctl restart mysql

## [MySQL] Rotate log file
mysql/rotate-log:
	-rm ${MYSQL_SLOW_LOG_PATH}
	# ファイルが更新されていることをMySQLに伝える
	systemctl restart mysql

## [MySQL] Install pt-query-digest
mysql/install-pt-query-digest:
	apt-get update
	apt-get install -y percona-toolkit

## [MySQL] Run pt-query-digest
mysql/pt-query-digest:
	pt-query-digest ${MYSQL_SLOW_LOG_PATH} > pt_query_digest_analysis.txt

## [MySQL] Run mysqldumpslow
mysql/mysqldumpslow:
	mysqldumpslow ${MYSQL_SLOW_LOG_PATH} > mysqldumpslow_analysis.txt

## [Nginx] Restart server
nginx/restart:
	nginx -t
	systemctl reload nginx

## [Nginx] Rotate log file
nginx/rotate-log:
	# 実行時点の日時を付与したファイル名にローテートする
	-mv ${NGINX_ACCESS_LOG_PATH} ${NGINX_ACCESS_LOG_PATH}.$(date +%Y%m%d-%H%M%S)
	# nginxにログファイルを開き直すシグナルを送信する
	nginx -s reopen

## [Nginx] Install alp
nginx/install-alp:
	apt install unzip
	wget https://github.com/tkuchiki/alp/releases/download/v1.0.9/alp_linux_amd64.zip
	unzip alp_linux_amd64.zip
	sudo install ./alp /usr/local/bin

## [Nginx] Run alp
nginx/alp:
	# パスパラメータの正規表現の例： -m "/posts/[0-9]+,/image/.*"
	# 並び替え： --sort=sum --sort=avg
	alp json --file ${NGINX_ACCESS_LOG_PATH} -m "/api/condition/.*,/api/isu/.*/icon,/api/isu/.*/graph,/api/isu/.*" -r > alp_analysis.txt

## [Redis] Install Redis
redis/install:
	apt-get update
	apt-get install -y redis
