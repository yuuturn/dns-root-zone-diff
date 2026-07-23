# General
日本語で会話します。
作りたいものは、DNS root zone file change notificationです。
つまりDNS root zoneを6時間に一度に取得して差分チェックして、差分はXやslackなどに通知するといった物になります。

## 実装について
実装は、Go言語で実装したいですが、他に良いものがあれば変更します。
コードはgitで管理し、remomte repoはGitHubとします。
テスト駆動開発(TDD)での開発手法を撮ります。
formatter/Linter,型チェックなどをgolangのbest practiceを利用したいです。
また、pre-commitでtestの通っていないものはcommitできず、GitHub ActionでもCIを回したい。

