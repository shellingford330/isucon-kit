# isucon-kit

## Prerequisites

- rsync

## Setup

### 1. Login as root user on remote host

`root`ユーザとしてログインできるようにする。

1. `isucon`ユーザとしてログイン

```sh
$ ssh isucon@{リモートホストIP}
```

2. `root`ユーザの`~/.ssh/authorized_keys`に公開鍵をコピー

```sh
$ sudo cp ~/.ssh/authorized_keys /root/.ssh/authorized_keys
```

### 2. Configure remote host

`~/.ssh/config`にホスト`isuconapp`と `isucondb` エイリアスを登録。

```
Host isuconapp
  Hostname <ホストIP>
  User root
  Port 22
  IdentityFile ~/.ssh/id_ed25519
Host isucondb
  Hostname <ホストIP>
  User root
  Port 22
  IdentityFile ~/.ssh/id_ed25519
```

### 3. Pull remote files

リモートホストのアプリケーション、Nginx, MySQL 設定ファイルをプルしてくるセットアップシェルスクリプトを実行。

```sh
$ sh ./pull.sh
$ sh ./install_tools.sh
```

### 4. Fix to restart application server

Makefile の app/restart と app/build の両方を修正する。

### 5. Set secrets in GitHub Actions

GitHub Actions の Secrets に Hostname と SSH キーの秘密鍵をセットする

```
APP_HOST_NAME=
DB_HOST_NAME=
SSH_KEY=
```

## Deploy

### 1. Build

```
$ make app/build
```

### 2. Push & Create PR

ファイルの変更を Commit & Push し、PR を作成すると自動でデプロイされる

## Analyze

Pull Request か Issue で下記のコメントをすると、アクセスログ解析とスロークエリログ解析が開始し、結果が出力させる

```
/analyze
```
