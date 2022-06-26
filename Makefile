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

## [MySQL] Rotate log file
mysql/rotate_log:
	rm /var/log/mysql/mysql-slow.log
	# ファイルが更新されていることをMySQLに伝える
	mysqladmin flush-logs

## [Nginx] Rotate log file
nginx/rotate_log:
	# 実行時点の日時を付与したファイル名にローテートする
	mv /var/log/nginx/access.log /var/log/nginx/access.log.$(date +%Y%m%d-%H%M%S)
	# nginxにログファイルを開き直すシグナルを送信する
	nginx -s reopen
