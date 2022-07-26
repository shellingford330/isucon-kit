# isucon-kit

## Prerequisites

- rsync

## Setup

### 0. Login As Root User On Remote Host

`root`ユーザとしてログインできるようにする。

1. `isucon`ユーザとしてログイン

```sh
$ ssh isucon@{リモートホストIP}
```

2. `root`ユーザの`~/.ssh/authorized_keys`に公開鍵をコピー

```sh
$ sudo cp ~/.ssh/authorized_keys /root/.ssh/authorized_keys
```

### 1. Configure Remote Host

`~/.ssh/config`にホスト`isucon`としてリモートホスト情報（root ユーザ）を登録。

```
Host isucon
  Hostname <ホストIP>
  User root
  Port 22
  IdentityFile ~/.ssh/id_ed25519
```

### 2. Pull Remote Files

リモートホストのアプリケーション、Nginx, MySQL 設定ファイルをプルしてくるセットアップシェルスクリプトを実行。

```sh
$ sh ./setup.sh
```
