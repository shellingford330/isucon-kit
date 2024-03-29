SHELL:=/bin/bash -e -o pipefail

MYSQL_SLOW_LOG_PATH:=/var/log/mysql/mysql-slow.log
NGINX_ACCESS_LOG_PATH:=/var/log/nginx/access.log

## [App] Restart server
app/restart:
	systemctl daemon-reload
	systemctl restart isucondition.go.service

## [App] Build
app/build:
	cd webapp/go && GOOS=linux GOARCH=amd64 go build -o isucondition

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
	alp json --sort=avg --file ${NGINX_ACCESS_LOG_PATH} -m "/api/isu/[a-z0-9-]+/graph,/api/condition/[a-z0-9-]+" -r > alp_analysis.txt

