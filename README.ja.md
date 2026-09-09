# tfc-vault

Terraform の API トークンを平文ファイルではなく、ロックされた macOS キーチェーンに
置き、必要なコマンドの実行中だけ Terraform に渡すツールです。
[aws-vault](https://github.com/99designs/aws-vault) と同じ発想で作っています。

`terraform login` はトークンを `~/.terraform.d/credentials.tfrc.json` に平文 JSON
で書き出します。パーミッションは 0600 ですが暗号化はされていないので、バックアップ、
同期されたホームディレクトリ、`cat` 一発で中身が出ます。さらに、自分と同じユーザーで
動くプロセスならいつでも読めます。

tfc-vault は専用のキーチェーンを作り、スリープ時と 15 分の無操作で再ロックします。
ロックされている間はトークンを誰も読めず、Terraform が必要としたときに macOS が
キーチェーンのパスフレーズを尋ねます。

macOS 専用です。他のバックエンドはありません。

## インストール

```sh
brew install iwashitahga/tap/tfc-vault
```

または

```sh
go install github.com/iwashitahga/tfc-vault@latest
```

続けて Terraform に組み込みます。

```sh
tfc-vault install
```

credentials helper のリンクを作り、`~/.terraformrc` に `credentials_helper`
ブロックを書き、既存の平文トークンをキーチェーンへ移すか尋ねます。完了後は
ラッパーなしで `terraform plan` がそのまま動きます。

## 使い方

### トークンを預ける

```sh
tfc-vault add                      # app.terraform.io のトークン
tfc-vault add -open                # 先にトークン発行ページをブラウザで開く
tfc-vault add tfe.example.com      # 自ホストの Terraform Enterprise
tfc-vault add -profile personal    # 同じホストに二本目のトークン
```

最初の `add` で `tfc-vault` キーチェーンが作られ、それを保護するパスフレーズを
尋ねられます。以降 macOS が求めてくるのはこのパスフレーズです。適当なダイアログに
反射で打ち込んでよいものではないので、そのつもりで決めてください。

トークン自体は echo されず、保存前にホストへ問い合わせて有効性を確認します。確認を
飛ばすなら `-no-verify` を付けます。

既に `terraform login` 済みなら平文ファイルから移せます。

```sh
tfc-vault import           # コピーするだけ。ファイルは残る
tfc-vault import -purge    # コピーしたあと上書きして削除
```

一度平文でディスクに置かれたトークンなので、移した後に web UI で再発行するのが
安全です。

### 一コマンドだけトークンを渡す

```sh
tfc-vault exec app.terraform.io -- terraform plan
tfc-vault exec -tfe-token app.terraform.io -- terraform plan
```

`TF_TOKEN_<host>` を子プロセスの環境に入れ、`exec(2)` で自分を置き換えます。自分の
シェルには何も残りません。`-tfe-token` を付けると `TFE_TOKEN` も設定します。
`tfe` プロバイダなど go-tfe 系のツールが読む変数です。フラグを付けない場合は
`TFE_TOKEN` を子プロセスから取り除くので、シェルに残っていた古い値がコマンドへ
漏れることはありません。

`-profile` を付けずに保存したプロファイルはホスト名そのものが名前になるので、
たいていはホスト名を指定すれば通ります。

### Terraform から取りに来てもらう

`tfc-vault install` の後は、Terraform が必要なときに tfc-vault を呼びます。
`terraform login` の保存先もキーチェーンになります。

ただし Terraform は credentials helper より先に `credentials.tfrc.json` を読み、
そのホストの記載があれば helper を呼びません。平文ファイルが残っている限り helper
は何もしません。`tfc-vault install` はこれを検知して移行を提案します。

外すときは `tfc-vault install -uninstall` を実行し、`~/.terraformrc` の
`credentials_helper` ブロックを手で消します。

### ロック

```sh
tfc-vault lock      # 今すぐ再ロック
```

キーチェーンはスリープ時と 15 分の無操作で再ロックします。他に何かする必要はありません。
ロックされた状態でトークンが読まれると macOS がパスフレーズダイアログを出し、答えれば
コマンドはそのまま続きます。

macOS に聞かせるのは意図的です。ダイアログは別のシステムプロセスが描画するので、
パスフレーズが tfc-vault のメモリに入りません。どのアプリケーションが要求しているかも
システムが表示します。tfc-vault が端末に出すプロンプトにはどちらの性質もなく、同じ行を
出力するだけのプログラムなら誰でも真似できます。

`tfc-vault unlock` は端末でパスフレーズを読みます。ssh など macOS がダイアログを
描画できない環境のためのもので、ダイアログが出せるならそちらを使ってください。

解錠したままにする時間を変えるときは、標準のキーチェーンコマンドを使います。これは
1 時間に設定し、スリープ時のロックは残します。

```sh
security set-keychain-settings -l -u -t 3600 ~/Library/Keychains/tfc-vault.keychain-db
```

二つ目のパスフレーズを管理したくない場合は `TFC_VAULT_KEYCHAIN=login` で login
キーチェーンを使えます。ただし保護は弱くなります。後述します。キーチェーン間の移動は
次のとおりです。

```sh
tfc-vault migrate -from login -remove
```

### その他

```sh
tfc-vault list        # プロファイル名の一覧
tfc-vault list -v     # ホストと環境変数名も表示
tfc-vault get NAME    # 生のトークンを標準出力へ
tfc-vault env NAME    # export 行を出力
tfc-vault remove NAME # プロファイルを削除
```

## 守れるものと守れないもの

トークンが平文でファイルシステム上に存在しなくなるので、バックアップや同期された
ホームディレクトリ、dotfile リポジトリに載ることがなくなります。

より重要なのは、普段ロックされているキーチェーンに入ることです。ロック中はトークンを
誰も読めません。別プロセスも、自分と同じ権限で動くスクリプトも、これもです。

```sh
security find-generic-password -s tfc-vault -a app.terraform.io -w
```

これは login キーチェーンでは得られない性質です。login キーチェーンはログイン時に
解錠されてセッション中ずっと開いたままで、項目ごとのアクセス制御リストは、保存した
バイナリに署名があってもなくても、実測では他のアプリケーションを制限しませんでした。
`TFC_VAULT_KEYCHAIN=login` にすると、平文ファイルと同じく、自分と同じユーザーで動く
任意のプロセスがいつでもトークンを読めます。

専用キーチェーンでも解錠中は同じことが言えるので、自動ロックの間隔がそのまま危険な
時間の長さになります。短くしたければ設定を縮めてください。

`tfc-vault exec` と `TF_TOKEN_*` の仕組み上、トークンは子プロセスの環境に入ります。
provider プラグイン、`local-exec` スクリプト、その他 Terraform が起動する
サブプロセスはすべて読めます。これは Terraform がトークンを受け取る方式そのものの
性質で、`aws-vault exec` と同じトレードオフです。

## コード署名

リリースバイナリには ad-hoc 署名が付いているので `codesign -v` は通りますが、
notarize はしていません。notarize には有料の Apple Developer Program が必要です。
そのため Releases からダウンロードしたバイナリは、隔離属性を外すまで Gatekeeper に
拒否されます。

```sh
xattr -dr com.apple.quarantine ./tfc-vault
```

Homebrew の cask はこれをインストール時に自動で行います。ソースからビルドした場合は
隔離属性が付かないので影響ありません。

## 関連ツール

Terraform のトークンをキーチェーンに保存する credentials helper は既にいくつかあり、
helper だけが必要ならそちらのほうが合うかもしれません。

- [bendrucker/terraform-credentials-keychain](https://github.com/bendrucker/terraform-credentials-keychain) — Go 製で、署名と notarize 済みのリリースがあります。
- [alisdair/terraform-credentials-keychain](https://github.com/alisdair/terraform-credentials-keychain) — シェルスクリプト実装です。
- [terracreds](https://github.com/tonedefdev/terracreds) — Windows と macOS 対応で、外部 vault バックエンドも持ちます。
- [terraform-credentials-op](https://github.com/razorsedge/terraform-credentials-op) — 1Password 版です。

tfc-vault が足しているのは、`aws-vault` 相当の `exec`、名前付きプロファイル、そして
手で設定するのではなく最初から用意されてロックされる専用キーチェーンです。

## 設計メモ

- トークンは `tfc-vault` キーチェーンのサービス名 `tfc-vault` 配下に、プロファイル
  ごとに一項目、`{"host": …, "token": …}` の形で入ります。
- 環境変数名はホスト名から導出します。`.` は `_`、`-` は `__`、国際化ドメインは
  punycode に変換します。`tfe.example-corp.com` なら
  `TF_TOKEN_tfe_example__corp_com` です。
- 同じホストを指すプロファイルが複数あると helper は選べません。
  `TFC_VAULT_PROFILE` で指定してください。
- ロックされたキーチェーンへの問い合わせは、結果ゼロ件を返します。トークンが無い
  場合と区別がつきません。tfc-vault はそれを信じる前にロック状態を確認するので、
  実際にはあるのに「認証情報が無い」と Terraform に伝えることはありません。
- キーチェーンへのアクセスには cgo が要ります。`CGO_ENABLED=0` のビルドは実行時に
  失敗するバイナリを作る代わりに、コンパイル時に拒否します。

Terraform 1.11.3 で、`exec` が注入する環境変数と credentials helper が返す
トークンの両方が実際の認証に使われることを確認しています。

## 開発

```sh
make build
make test
```

## ライセンス

MIT
