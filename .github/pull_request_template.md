## Check

- [ ] 最新の main ブランチを取り込み済み

```sh
$ git fetch orign main
$ git rebase origin/main
```

- [ ] マージ条件（以下のどれかを満たしていればマージして良い）
  - [ ] ベンチマークのスコアが上がった
  - [ ] CPU 使用率に変化があった
  - [ ] ログ解析で改善が見られている
