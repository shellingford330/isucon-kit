# isucon-kit

## Prerequisites

1. `~/.ssh/config`にホスト`isucon`としてリモートホスト情報を登録

```
Host isucon
  Hostname <ホストIP>
  User <ログインするユーザ>
  Port 22
  IdentityFile ~/.ssh/id_ed25519
```

2. 環境変数を`.envrc`にセットする

```
export APP_PATH=/home/isucon/private_isu/webapp/golang/
export NGINX_CONF_PATH=/etc/nginx/nginx.conf
export MYSQL_CONF_PATH=/etc/mysql/conf.d/mysql.cnf
```
