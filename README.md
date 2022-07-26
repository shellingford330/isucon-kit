# isucon-kit

## Prerequisites

- ssh
- rsync

## Setup

### 1. Configure Remote Host

`~/.ssh/config`にホスト`isucon`としてリモートホスト情報を登録。

```
Host isucon
  Hostname <ホストIP>
  User isucon
  Port 22
  IdentityFile ~/.ssh/id_ed25519
```

### 2. Set Environment Variables

`.envrc`ファイルに環境変数をセットする。

```
export APP_PATH=/home/isucon/webapp/
export NGINX_CONF_PATH=/etc/nginx/
export MYSQL_CONF_PATH=/etc/mysql/
```

### 3. Pull Remote Files

リモートホストのアプリケーション、Nginx, MySQL 設定ファイルをプルしてくるセットアップシェルスクリプトを実行。

```sh
$ sh ./setup.sh
```
