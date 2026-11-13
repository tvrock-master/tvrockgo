# タグからのリリース

開発者向けの公開手順です。配布版の導入・更新は[README](../README.md)、ビルド環境は[AGENTS.md](../AGENTS.md)を参照してください。タグのpushはRelease公開につながるため、配布内容と検証結果を確認してから実行します。

`v1.2.3` または `v1.2.3-rc.1` のようなSemVerのタグをpushすると、GitHub ActionsでWindows x64版をビルドし、`tvrockgo.zip` をReleaseへ添付する。

```powershell
git tag v1.2.3
git push origin v1.2.3
```

ZIPにはサーバー、Webアプリ、RockbarのNSISインストーラー、初期設定、テンプレート、サービス配置ひな形（`deploy/`）、空のdata／logディレクトリを含む。配布するドキュメントは、ライセンス各種と次の一覧に限定する。`buildall.ps1`でファイル名を明示し、`docs/`全体はコピーしない。

- ルートの`README.md`と`TvRockGO-Manual.pdf`
- `docs/custom-command-examples/`の全内容
- `docs/images/`の全内容
- `docs/configuration.md`
- `docs/custom-commands.md`
- `docs/playback.md`
- `docs/remote-viewing.md`
- `docs/rockbar.md`
- `docs/startup.md`
- `docs/tvguide.md`

`buildall.ps1`は、配布対象のMarkdownと画像から目次付きの`TvRockGO-Manual.pdf`を生成し、`README.md`と同じ階層に置いてからZIPを作成する。GitHub Actions、GitLab CI、手動の配布ビルドで共通の処理を使う。PDF生成にはRockbarのビルド用Electronと、`scripts/release-manual/package-lock.json`で固定したMarkdown変換ライブラリ・日本語フォントを使用する。

ルートの`LICENSE`・`THIRD_PARTY_LICENSES.md`と、Webアプリ・Rockbarのビルドに含まれる依存ライブラリやアイコン等のライセンス表記は保持する。Electronは1項目にまとめ、内包部品の原文はElectron付属の`LICENSE`・`LICENSES.chromium.html`を`licenses/electron/`へコピーする。Windows版Rockbar単体にも、Electron付属のライセンスと`resources/THIRD_PARTY_LICENSES.md`を同梱する。`rockbar/README.md`、`AGENTS.md`、`docs/releases.md`など、上の一覧にない文書は同梱しない。個人用の`conf/tvrock.yml`、実DB、ログ、Rockbarの展開済み作業フォルダーも含めない。

### ライセンス表記の更新

依存を更新した場合は、通常のGo・npm依存を取得したうえで、次をルートから実行する。Electron本体はnpm依存のインストールだけでは取得されないため、付属のインストールスクリプトを明示的に実行する。`buildall.ps1`でもライセンス検査の前に同じ取得処理を行うため、GitHub Actions・GitLab CI・手動ビルドで共通して適用される。Webアプリの本番バンドルのレポートから、ローダー等のビルド専用コードと、実際に出力されるコードを区別する。GoはWindows・Linux・macOSの実行時依存を調べる。運用サーバーは起動しない。

```powershell
node rockbar/node_modules/electron/install.js
npm run build --prefix app -- --dest ../.tmp/license-app-build --report-json
node scripts/generate-third-party-licenses.cjs --app-report .tmp/license-app-build/report.json
node scripts/generate-third-party-licenses.cjs --check
```

生成する`THIRD_PARTY_LICENSES.md`・`template/login.html`・`licenses/manifest.json`を一緒に更新する。上流のNOTICE・COPYING・著作権表示も収録し、Go・npmの依存でライセンスが見つからない場合は黙って省略せず生成を失敗させる。ログイン画面の本文はHTMLとGoテンプレートの構文をエスケープして埋め込む。`--check`は依存ファイルと生成元のライセンスをファイル単位で照合し、変更・欠落があればパスを表示する。`buildall.ps1`でも配布ビルドの前に実行する。

CDNのフォントとNSISプラグインの原文・取得元は`licenses/`で管理する。CDNの配信版やインストーラー用プラグインを変更した場合は、これらも上流と照合して更新する。Electron内包部品は展開して重複掲載せず、Electronが提供する全文ファイルを改変せず同梱する。

`rockbar/Rockbar-Setup-<バージョン>-x64.exe` を実行するとインストール画面が開く。インストール後はスタートメニューから起動し、Windowsの「インストールされているアプリ」からアンインストールできる。設定はユーザーのAppDataに保存し、更新・アンインストール時にも保持する。既存のNSIS版は新しいインストーラーで更新する。更新時に同じアプリとして認識させるため、`appId` は `jp.tvrockgo.rockbar` を維持する。従来のポータブル版はインストール登録がないため、初回インストールとなる。

初期設定ひな形のカスタムコマンドのサンプルは、同梱する`docs/custom-command-examples/`のスクリプトを参照する。

## バージョン

ビルド時に `APP_VERSION` にタグを渡す。先頭の `v` を外した完全なバージョンをGoサーバーとRockbarに使用する。Webアプリの表示にはタグを使用する。

Rockbarの `package.json`、`package-lock.json` 本体、`packages[""].version` をビルド用チェックアウト内で更新する。Electronのパッケージにも同じバージョンが入り、EXE名は `Rockbar-Setup-1.2.3-rc.1-x64.exe` となる。バージョン更新のコミットは自動作成しない。

不正なタグではビルドを失敗させる。以前のようにバージョンを `1.0.0` に戻したり、プレリリース部分を切り捨てたりしない。

ローカルでの確認:

```powershell
npm ci --prefix app
npm ci --prefix rockbar
$env:APP_VERSION = 'v1.2.3'
./buildall.ps1
```

このビルドは`tvrockgo/`と`rockbar/dist/`を再作成し、Webアプリのビルドで`static/`も作り直す。運用設定・DB・局ロゴのある稼働用フォルダでは実行せず、公開用のソースチェックアウトで確認する。`APP_VERSION`を指定するとRockbarの`package.json`と`package-lock.json`も更新する。

成果物は `tvrockgo/` と `.tmp/release/tvrockgo.zip` に生成する。タグを指定しないローカルビルドでは、Rockbarの現在の `package.json` のバージョンを使用する。

## CI

- GitHub: ブランチpushでGoテスト・静的チェック、ライセンス表記の検査、Rockbarテストを実行する。検査に通ったコミットにタグを付ける。タグpushでは `build` を実行し、配布ビルドの開始時にライセンス表記を再検査する。ビルドジョブだけに `contents: write` を付与し、`GITHUB_TOKEN` でReleaseを作成する。既存ReleaseならZIPを差し替え、リリースノートを維持する。
- GitLab: タグpushで `test` → `build`。`saas-windows-medium-amd64` Runnerを使用する。独自Runnerを使う場合は `.gitlab-ci.yml` のタグを変更する。
- GitLabでは `setup-ci.ps1` が必要なGoとNode.js 22を用意し、取得したZIPのSHA256を公式配布元の値で照合する。
- GitLabの `CI_JOB_TOKEN` でGeneric Package RegistryへZIPを保存し、ReleaseのAssetsにリンクを追加する。再実行時はこのZIPのリンクだけ更新する。
- 認証エラー・APIサーバーエラーでは公開処理を停止する。Releaseが404の場合だけ新規作成する。
- GitHubの自動実行はブランチpushとタグpushのみ。GitLabでは従来どおりタグpushでテストしてからビルド・公開する。

参考: [GitLab Windows Runner](https://docs.gitlab.com/ci/runners/hosted_runners/windows/)、[GitLab Release API](https://docs.gitlab.com/api/releases/)、[GitHub Release API](https://docs.github.com/en/rest/releases/releases)。
