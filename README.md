# isucon-kit

## Prerequisites

- ssh
- rsync
- direnv

### 1. Configure Remote Host

`~/.ssh/config`にホスト`isucon`としてリモートホスト情報を登録。

```
Host isucon
  Hostname <ホストIP>
  User <ログインするユーザ>
  Port 22
  IdentityFile ~/.ssh/id_ed25519
```

### 2. Set Environment Variables

`.envrc`ファイルに環境変数をセットする。

```
export APP_PATH=/home/isucon/private_isu/webapp/golang/
export NGINX_CONF_PATH=/etc/nginx/nginx.conf
export MYSQL_CONF_PATH=/etc/mysql/conf.d/mysql.cnf
```

### 3. Pull Remote Files

リモートホストのアプリケーション、Nginx, MySQL 設定ファイルをプルしてくるセットアップシェルスクリプトを実行。

```sh
$ sh ./setup.sh
```

## Deploy

リモートホストにデプロイする。

```sh
$ sh ./deploy.sh
```

## Analyze Access Log

```sh
% make nginx/install-alp
% make nginx/alp
```

## Analyze Slow Query Log

```sh
% make mysql/install-pt-query-digest
% make mysql/pt-query-digest
```
