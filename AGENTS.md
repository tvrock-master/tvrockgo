# AGENTS.md

## 目的と作業方針

TvRockGOのソース構成と開発手順をまとめた、開発者・AIエージェント向けの指示書です。導入・運用手順は`README.md`を参照してください。このファイルはリポジトリのルートに置きます。

- 作業は現在のプロジェクトフォルダで行い、実プロジェクトを移動・複製しないでください。
- 別の一時作業フォルダや手動worktreeが必要な場合だけ、環境で指定されたAI作業用領域を使用してください。個人環境の絶対パスを公開ファイルに記載しないでください。
- 個人用設定・DB・ログ・録画ファイルを変更・削除せず、実機や稼働中のサーバーを使う検証はユーザーの依頼範囲を確認してください。
- 公開用の整理では現在のファイルを対象とし、Git履歴の書き換えや公開操作は依頼がある場合だけ行います。

## プロジェクト構成

| パス | 役割 |
| --- | --- |
| `main.go` | 引数解析、保存先の決定、共通の初期化・サーバー起動・終了処理、`--timemachine`コマンド |
| `startup.go` / `startup_windows.go` / `startup_unix.go` | ローカルURL生成、Windowsのトレイ・コンソール制御、Linux/macOSのサービス停止待機 |
| `tray_icon_windows.go` / `rockbar_windows.go` | 通常・スリープ用アイコンの埋め込みと変換、Rockbarの自動検出・起動 |
| `single_instance_*.go` | OS別の多重起動防止 |
| `tvrock/` | Goバックエンド。録画・視聴アプリ制御、スケジューラー、番組情報、HTTP API、WebSocket |
| `app/` | Vue 2・Vuetify 2による番組表・予約・検索・設定画面 |
| `rockbar/` | 共通のHTML/JSから作るElectron版・ブラウザ版コントローラー |
| `conf/tvrock.org.yml` | 公開用の初期設定ひな形 |
| `conf/tvrock.yml` | 初回起動時に生成される運用設定（公開対象外） |
| `template/` | ログイン・エラーページのHTMLテンプレート |
| `static/` | Webアプリのビルド出力。受信した局ロゴは`static/cdt/` |
| `data/` | SQLite DBとBleve検索インデックス（公開対象外） |
| `log/` | 実行ログ（公開対象外） |
| `docs/` | 設定・起動・リリース等の資料と、`images/`内のREADME用スクリーンショット |
| `deploy/` | systemd用unit・launchd用plistの配置ひな形 |
| `.github/workflows/` | ブランチpushの検証、タグpushのWindows配布ビルド |
| `.gitlab-ci.yml` | GitLabでのタグ向け検証・配布ビルド |
| `build.ps1` | Windows x64のGUI形式トレイ常駐プログラムのビルド |
| `build-service.ps1` | Linux/macOSサービス用プログラムのクロスビルド |
| `buildall.ps1` | サーバー・Webアプリ・Rockbarのビルドと配布ZIP作成 |
| `setup-ci.ps1` | GitLab CI用Go・Node.jsセットアップ |
| `publish-*-release.ps1` | CIのRelease公開処理 |

`tvrockgo/`は配布物の組み立て先です。`tvrock/`はソースフォルダなので、取り違えて削除しないでください。

## バックエンドの主な責務

- `configure.go` / `init.go`：YAML設定、初回設定生成、認証情報のハッシュ化、各機能の初期化。
- `webserver.go` / `api.go` / `controller.go` / `login.go` / `session.go`：HTTP、WebSocket、認証、ブラウザ操作。
- `launcher.go` / `deviceserver.go` / `rockdevice.go`：録画アプリの起動・終了、接続と受信データ。
- `scheduler.go` / `task*.go` / `keywordreserve.go` / `fetchprogramscheduler.go`：予約、録画タスク、キーワード予約、EPG取得。
- `database.go` / `station.go` / `tvprogram.go` / `archiver.go`：局・番組・予約・字幕等の保存とアーカイブ。
- `search*.go` / `bleve.go`：番組・字幕検索、全文検索インデックス。
- `caption.go` / `cdt.go` / `dsm*.go` / `bml.go`：字幕、局ロゴ、DSM-CC、BML。
- `powermanager_*.go`：OS別のスリープ抑止・復帰・電源管理。
- `powermanager_setting.go`：電源管理設定のスナップショット取得、トレイ・API共通の保存・失敗時の復元・クライアント通知。
- `exec_options_windows.go` / `exec_options_unix.go`：補助コマンドのOS別起動オプション。Windowsは`CREATE_NO_WINDOW`、Linux/macOSは設定を変更しない。

主要依存はGin、Gorilla WebSocket/Sessions、modernc.org/sqlite、Bleve、cronです。正確なバージョンは`go.mod`を参照してください。専用`TvRockDTV.dll`や外部録画アプリのソースはこのリポジトリには含みません。

## 開発環境

- Windows x64、PowerShellを基本とします。サーバーにはLinux/macOS向けのOS別実装もありますが、Windowsの録画環境と同等の実機動作を保証するものではありません。
- Go：`go.mod`で指定する1.26.3以上。
- Node.js：CIでは22を使用。Rockbarの開発資料も22以上を前提とします。npmを使用し、依存の固定は各`package-lock.json`を基準にします。
- VS CodeでGoをデバッグする場合はGo拡張とDelveが必要です。
- 初回の依存取得、Windowsアイコン生成、Electronビルドにはネットワーク接続が必要です。

ルートで依存を取得します。

```powershell
go mod download
npm ci --prefix app
npm ci --prefix rockbar
```

## ビルド

### サーバー

ルートから実行します。

```powershell
.\build.ps1
```

`GOOS=windows`、`GOARCH=amd64`、`CGO_ENABLED=0`、`-trimpath`、`-H=windowsgui`で`tvrockgo.exe`を作成します。`go-winres`で`app/public/image/tvrockgo.png`から実行ファイルのアイコン用`.syso`も生成します。`APP_VERSION`未指定時のサーバーバージョンは`dev`です。

通常の配布ビルドには`build.ps1`を使用してください。一時ファイルとGoキャッシュを`.tmp/`に配置します。

運用中の実行ファイルを上書きせずビルドだけ検証する場合は、別の出力先を指定します。

```powershell
.\build.ps1 -OutputPath '.tmp/verify-windows/tvrockgo.exe'
```

これは実行ファイルだけの出力です。実行するには、その実行ファイルと同じ場所に`static/`と`template/`が必要です。`--data-dir`はこれらの配置先を変更しません。ビルド確認のために実設定・DBを使ってサーバーを起動しないでください。

### Linux/macOSサービス版

```powershell
.\build-service.ps1 -TargetOS linux -Arch amd64
.\build-service.ps1 -TargetOS darwin -Arch arm64
```

`-TargetOS`は`linux`・`darwin`、`-Arch`は`amd64`・`arm64`に対応し、出力は`.tmp/service/<OS>-<Arch>/tvrockgo`です。`CGO_ENABLED=0`・`-trimpath`でビルドし、`APP_VERSION`をバージョンに反映します。リソースのコピーやサービス登録は行いません。`deploy/`のひな形と`docs/startup.md`を参照し、配置先・実行ユーザー・権限・HTTPポートを環境に合わせて設定してください。

### WebアプリとRockbar

```powershell
npm run build --prefix app
npm run build:web --prefix rockbar
```

`app/vue.config.js`によりWebアプリを`static/`へ出力し、Rockbarのブラウザ版を`static/rockbar/`へ追加します。**app → Rockbarの順にビルド**してください。appのビルドは`static/`をクリーンするため、既存の`static/cdt/`やRockbar出力を消します。運用フォルダでは事前にバックアップするか、`npm run build --prefix app -- --dest ../.tmp/frontend-preview`のように別の出力先を使ってください。

```powershell
npm run build:electron --prefix rockbar
```

Windows版Rockbarは`rockbar/dist/Rockbar-Setup-<バージョン>-x64.exe`に出力します。`npm run build:all --prefix rockbar`でブラウザ版とWindows版の両方を作成できます。

### 配布物

```powershell
$env:APP_VERSION = 'v1.2.3'
.\buildall.ps1
```

`tvrockgo/`と`.tmp/release/tvrockgo.zip`を生成します。組み立て先と`rockbar/dist/`は再作成されます。`APP_VERSION`を指定するとRockbarの`package.json`と`package-lock.json`のバージョンも書き換わります。公開用設定はひな形だけをコピーし、運用設定・DB・ログは同梱しません。詳細は`docs/releases.md`を参照してください。

配布物には`docs/startup.md`と`deploy/`もコピーします。トレイの両アイコンは実行ファイルに埋め込まれるため、別のPNG・ICOの配布は不要です。

## テストと検証

ルートから実行します。

```powershell
go test ./...
go vet ./...
node --test app/tests/captionHistory.test.cjs app/tests/captionPlayback.test.cjs app/tests/captionWindow.test.cjs app/tests/frontendReview.test.cjs
npm test --prefix rockbar
```

Goの`TestMain`は一時ディレクトリに専用DBを作成します。実設定の読み込みや録画アプリの起動を行うテストへ変更しないでください。ブラウザテスト（`app/tests/captionHistoryLifecycle.test.cjs`、`app/tests/timeTableBrowser.test.cjs`、Rockbarの`*smoke.cjs`）には、通常のnpm依存に含まれないPlaywrightと対応ブラウザ等の追加環境が必要です。appの2件は`msedge`チャネルを指定するためMicrosoft Edgeも必要です。対象ファイルを読んで環境を用意した場合に実行してください。

フロントエンド変更時は`npm run build --prefix app`を確認します。局ロゴがある環境では前述の別出力先を使います。実機の録画・専用DLLとの接続・スリープ復帰は自動テストだけでは確認できません。

GitHub Actionsはブランチpush時にWindowsとUbuntuでGoテストと`go vet`を実行し、WindowsでRockbarをテストします。タグpush時はテストを再実行せず、Windows配布物をビルド・公開します。GitLab CIは従来どおりタグpush時にテスト後、ビルド・公開します。現在のタグワークフローは全タグで起動するため、リリースにはRockbarが受け付けるSemVer形式を使用してください。

トレイ関連の変更では`startup_windows_test.go`のクリック通知・メニュー条件・両PNGのアイコン生成を確認します。設定更新は`tvrock/powermanager_setting_test.go`の保存・通知・保存失敗時の復元、外部コマンドは`tvrock/exec_options_test.go`の標準出力・標準エラー・終了コード・ストレージ解析を確認します。いずれも通常の`go test ./...`に含まれます。Windows固有のテストはWindowsで実行し、共通コード変更時はLinux/macOSのクロスビルドも確認してください。ネイティブUIの実表示、実機のスリープ・復帰、サービス登録・起動は別途検証が必要です。

## 起動・デバッグ

- 実行ファイルと同じフォルダに`static/`と`template/`を配置します。設定・DB・ログなどの既定の基準ディレクトリは実行ファイルの配置先です。別の場所を使う場合は`--data-dir`で指定します。`--data-dir`・`--rockbar`・`--timemachine`の相対パスは、起動時の作業ディレクトリを基準に解決してから保存先へ移動します。設定ファイル内の相対パスは、この保存先を作業ディレクトリとして使います。
- Windowsは引数なしでトレイ常駐、`--console`でトレイなしのコンソール起動です。Linux/macOSは引数なしでUIなしのプロセスとして動作し、systemd/launchdに常駐と自動起動を管理させます。プロセス自身ではサービス登録やforkによるバックグラウンド化を行いません。
- `--timemachine <パス>`は取り込み完了後に終了します。Ctrl+Cで中断でき、保存済みの番組情報は保持します。`--data-dir`などのオプションも併用できます。`--help`や不正な引数では設定・DBを初期化しません。
- `--timemachine`は`ImportTimeMachine()`による専用の初期化・終了処理を使います。既存の運用設定を読み、局情報DBは読み取り専用、過去番組DBは書き込み用に開きます。通常の`Initialize()`・`Close()`を通さず、通常サービスや番組削除処理を起動しない構成を維持してください。
- ローカル運用設定は`conf/tvrock.yml`を使います。編集前にサーバーを終了してください。運用設定が存在しない場合だけ`conf/tvrock.org.yml`を読み込んで生成します。ひな形は設定の再生成時にも必要です。
- VS Codeの`.vscode/launch.json`からF5でサーバーをデバッグします。`--console --data-dir ${workspaceFolder}`を指定済みで、一時ファイルは`${workspaceFolder}/.tmp`を使います。
- `npm run serve --prefix app`でVue開発サーバーを起動できます。コード変更は自動反映されます。ただし現在のWebアプリはページのoriginをAPI/WebSocketの接続先とし、devServerにはAPIプロキシがありません。開発サーバーだけではGo APIへ接続できないので、通信を伴う確認にはサーバー配信のビルドを使用するか、開発用プロキシを構成してください。
- `npm start --prefix rockbar`でElectron版を開発起動します。

### トレイ・電源管理を変更するとき

- トレイはログインユーザーのプロセスで、Windowsサービスとは実行環境が異なります。ログイン前の稼働が必要な場合は、サービスとユーザー側UIを分離する設計が必要です。
- `main.go`はHTTPの待受成功後にランチャーとトレイを起動します。起動エラーは呼び出し元へ返し、終了はコンテキストのキャンセルと`tvrock.Close()`へ集約します。リソースを開いた後の`log.Fatal`などで正常終了処理を飛ばさないようにしてください。
- トレイの作成・メッセージ処理・アイコン変更は初期OSスレッドで実行します。電源監視のウィンドウとメッセージループ、スリープ抑止はそれぞれ専用OSスレッドに固定しています。電源管理はメインスレッド必須ではなく、ウィンドウの作成・処理・破棄を同じスレッドで行うことと、`SetThreadExecutionState`の設定・解除を同じスレッドで行うことが必要です。既存の`runtime.LockOSThread()`を維持してください。
- 通常アイコンは`app/public/image/tvrockgo.png`、スリープ用は`app/src/assets/tvrockgo_sleep.png`を`go:embed`し、起動時に32×32ピクセルのWindowsアイコンへ変換します。画像の変更・移動には埋め込み指定の更新と再ビルドが必要です。
- `PowerManager == true`のときだけスリープタイマーのメニューを表示し、`SleepTimerPause == false`でチェックします。専用アイコンの条件は`PowerManager && !SleepTimerPause`です。左クリックとメニューは同じ切り替え処理を使用します。Version 4では選択通知、旧方式ではマウス通知を扱い、一度のクリックを二重に切り替えないでください。
- 電源設定の読み取りは`CurrentPowerManagerSetting()`のスナップショットを使います。トレイからの切り替えは`ToggleSleepTimerPause()`、APIからの変更は共通の`updatePowerManagerSetting()`を使い、設定の直接書き換えを避けます。保存失敗時は元の設定へ戻し、成功後だけ`ControllerUpdatePowerManagerSetting`を通知します。`saveLocked()`は設定のロックを保持している場合だけ呼び出します。
- Windowsの補助コマンドには`configureBackgroundCommand()`で`CREATE_NO_WINDOW`を設定します。ストレージ監視・カスタムコマンドと`schtasks.exe`が対象で、`CombinedOutput()`による出力・終了コード・キャンセルの扱いを維持します。録画・視聴アプリやRockbarに非表示処理を一律適用しないでください。
- Rockbarは指定パス、インストール情報、同梱先・標準インストール先から検出し、不在・起動失敗時はブラウザ版へ切り替えます。インストーラーやアンインストーラーを検出候補に追加しないでください。

操作・サービス配置の詳細は`README.md`と`docs/startup.md`を参照してください。

## 公開ファイルの扱いとコードスタイル

Goは`gofmt`、JavaScript/Vueは既存スタイルに合わせます。実装と設定キー・サンプルを一致させ、挙動が変わる場合はREADMEや関連資料も更新してください。

- 公開サンプルのユーザー名・パスワード・アプリパス・ドライバ名は例示値を使います。
- 運用設定、セッションキー、パスワードハッシュ、DB、ログ、個人環境のパス、ローカル検証資料、`.cache/`・`test-output/`・`.agents/`は公開しません。`.gitignore`だけでなく、新リポジトリへのコピーやZIP作成時も確認してください。
- 依存ライブラリの著者名やライセンス表記、著作権表示は保持します。
- 現在のGoモジュール名は`go.mod`にある`github.com/tvrock-master/tvrockgo`です。`README.md`のGitHub公開予定先とは異なります。公開先に合わせてモジュール名を変更する場合は、`go.mod`、内部import、`build.ps1`と`build-service.ps1`の`-X`指定を一緒に更新してください。関連プロジェクトは`README.md`の公開予定先を参照してください。

## Codexの実行環境

Windowsの管理サンドボックスで、コマンド開始前に`windows sandbox: spawn setup refresh`が出ることがあります。これはプロジェクトの失敗ではありません。検査・整形・ビルド・テストに必要なコマンドは、同じコマンドを`sandbox_permissions: "require_escalated"`と短い日本語の理由付きで再実行してください。

承認プレフィックスの例：`["Get-Content"]`、`["rg"]`、`["gofmt"]`、`["go", "test"]`、`["npm", "run", "build"]`。作業フォルダの具体的な指定はセッション側の指示に従ってください。
