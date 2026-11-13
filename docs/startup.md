# 起動方式とサービス配置

導入と機能の概要は[README](../README.md)、デバイス・認証・電源管理の設定例は[設定ガイド](configuration.md)を参照してください。

## 共通処理とOS別の入口

`main.go`は引数解析、初期化、HTTP・予約・録画処理の起動と正常終了を共通で実行します。
Windowsでは`startup_windows.go`、Linux/macOSでは`startup_unix.go`だけがビルド対象になります。
同じソースツリーから、Windowsのトレイ常駐プログラムとLinux/macOSのサービスプログラムを個別にビルドできます。

以下のビルドコマンドはソースリポジトリのルートで実行します。

| 起動方法 | 動作 |
| --- | --- |
| Windowsの引数なし | コンソールを表示せず、トレイ常駐 |
| Linux/macOSの引数なし | UIなしのプロセス。サービス管理はsystemd/launchdが担当 |
| `--console` | トレイなし。標準出力にもログを表示し、Ctrl+Cで終了 |
| `--data-dir <ディレクトリ>` | 設定・DB・ログなどの基準ディレクトリを指定 |
| `--rockbar <実行ファイル>` | Windows版Rockbarの起動対象を優先指定 |
| `--timemachine <パス>` | 旧TvRockの番組データを取り込み、完了後に終了。Ctrl+Cで中断 |
| `--help` | ヘルプ表示。設定・DBの初期化なし |

設定・DB・ログの基準は**実行ファイルのディレクトリ**です。従来の作業ディレクトリ基準で運用していた場合は、`--data-dir`でそのディレクトリを指定してください。`static/`と`template/`は従来どおり実行ファイルと同じ場所に必要です。

```powershell
.\tvrockgo.exe
.\tvrockgo.exe --console
.\tvrockgo.exe --console --data-dir 'C:\TvRockGO-state'
.\tvrockgo.exe --rockbar 'C:\Apps\Rockbar\Rockbar.exe'
Start-Process -FilePath .\tvrockgo.exe -ArgumentList '--timemachine "C:\Import\tvrock"' -NoNewWindow -Wait
```

`--timemachine`は`--data-dir`などの他のオプションと併用できます。相対指定の`--data-dir`、`--rockbar`、取り込みパスは起動時の作業ディレクトリを基準に解決します。

取り込みでは既存の運用設定を読み、局情報DBを読み取り専用で参照し、過去番組DBだけを書き込み先として開きます。通常サービス・検索インデックス・番組削除処理は初期化せず、取り込み専用の接続を閉じて終了します。

## Windows

`build.ps1`でWindows x64 GUI形式の`tvrockgo.exe`を生成します。指定画像`app/public/image/tvrockgo.png`を実行ファイルのアイコン生成とトレイアイコンに使用します。トレイ用PNGは実行ファイルへ埋め込まれ、起動時に32ピクセルのWindowsアイコンへ変換されます。

トレイの右クリックメニューは「番組表を開く」「Rockbarを開く」「終了」と、電源管理が有効な場合の「スリープタイマー」です。HTTPのポートを確保した後にトレイを作成します。番組表は既定ブラウザで開き、Rockbarは`--rockbar`で指定した実行ファイルを優先し、未指定・不在の場合はWindowsのインストール情報、TvRockGOと同じ場所の`rockbar/Rockbar.exe`・`Rockbar.exe`、標準インストール先の順に自動検出します。インストール先を変更したアプリにも対応します。アプリが見つからない場合、または候補の起動がすべて失敗した場合はブラウザ版を開きます。Rockbarのサーバー接続設定はRockbar側で行います。インストーラーは起動対象に指定しないでください。

`--console`は親のコンソールに接続し、親にコンソールがなければ新しく作成します。GUI形式の実行ファイルをPowerShellで実行して終了を待ちたい場合は、次のようにします。

```powershell
Start-Process -FilePath .\tvrockgo.exe -ArgumentList '--console' -NoNewWindow -Wait
```

通常起動はログインユーザーのトレイ常駐です。自動起動は必要に応じてユーザーのスタートアップにショートカットを登録してください。Windowsサービスへの登録は行いません。ログオフ後も録画を継続する用途には対応しません。

トレイのメッセージループは初期OSスレッドで実行します。電源監視とスリープ抑止は既存の専用OSスレッドに固定し、トレイと分離しています。`SetThreadExecutionState`の設定・解除は同一スレッドで実行します。

`powermanagersetting.powermanager: true`の場合だけ「スリープタイマー」メニューを表示します。`sleeptimerpause: false`でチェックが付き、クリックすると一時停止・再開を切り替えて設定を保存します。保存に失敗した場合は元の設定へ戻してエラーを表示し、保存に成功した場合だけWeb画面・Rockbarへ変更を通知します。電源管理が有効で一時停止していない間は、`app/src/assets/tvrockgo_sleep.png`を埋め込んだアイコンを使い、それ以外は通常のアイコンを使います。Web画面からの変更はアイコンへ約1秒以内に反映し、メニューのチェックは開くたびに最新設定から作成します。

ストレージ監視・カスタムコマンドと復帰タスク用の`schtasks.exe`は、Windows専用の`exec_options_windows.go`で`CREATE_NO_WINDOW`を設定して実行します。コンソールウィンドウだけを抑止し、`CombinedOutput()`による標準出力・標準エラーの取得と終了コードの判定は維持します。Linux/macOS用の`exec_options_unix.go`はプロセス設定を変更しません。録画・視聴アプリとRockbarの起動にはこの設定を適用しません。

トレイアイコンの左クリックでもスリープタイマーの一時停止・再開を切り替えられます。メニューと同じ保存・通知処理を使用し、アイコンも更新します。電源管理が無効な場合は変更しません。右クリックはマウス位置を基準にメニューを表示します。通知領域の隠れたアイコン一覧から開いた場合も、メニューの所有ウィンドウを前面化し、最前面グループ内の順序を更新してから表示します。

## Linux

Windows上でのクロスビルド例です。

```powershell
.\build-service.ps1 -TargetOS linux -Arch amd64
```

出力は`.tmp/service/linux-amd64/tvrockgo`です。Linux上で直接ビルドする場合は`CGO_ENABLED=0 go build -trimpath -o tvrockgo .`を使用できます。

`deploy/tvrockgo.service`はsystemd用のひな形です。次の条件に合わせて、配置パス・ユーザー名・グループ名を編集してから登録します。

- 実行ファイル、`static/`、`template/`、`conf/tvrock.org.yml`を`/opt/tvrockgo`へ配置する。
- クロスビルドした実行ファイルには、配置先で`chmod +x /opt/tvrockgo/tvrockgo`を実行する。
- 専用ユーザー`tvrockgo`とグループを用意し、設定・DB・ログ・録画先に必要な読み書き権限を与える。
- `data/`と`log/`を用意する。既定の80番ポートは一般ユーザーでは利用できない場合があるため、設定の`host`を`localhost:8080`などに変更する。接続URLを変えた場合は認証・Origin設定も合わせる。
- ひな形を`/etc/systemd/system/tvrockgo.service`へ配置し、`sudo systemctl daemon-reload`、`sudo systemctl enable --now tvrockgo`を実行する。

停止は`sudo systemctl stop tvrockgo`、状態確認は`systemctl status tvrockgo`です。アプリのファイルログに加えて標準出力をjournalへ送ります。停止待機の上限はひな形では120秒です。運用に応じて調整してください。

## macOS

```powershell
.\build-service.ps1 -TargetOS darwin -Arch arm64
```

出力は`.tmp/service/darwin-arm64/tvrockgo`です。Intel Mac向けは`-Arch amd64`を使用します。

`deploy/jp.tvrockgo.server.plist`はシステム用LaunchDaemonのひな形です。専用ユーザー`tvrockgo`を用意し、実行ファイルとリソースを`/usr/local/lib/tvrockgo`へ配置します。ユーザー名・パスは環境に合わせて変更してください。`data/`と`log/`を作成し、専用ユーザーに必要なアクセス権を与えます。Linuxと同様、必要に応じてHTTPポートと関連設定を変更します。

クロスビルドした実行ファイルには、配置先で`chmod +x /usr/local/lib/tvrockgo/tvrockgo`を実行します。

管理者がひな形を`/Library/LaunchDaemons/jp.tvrockgo.server.plist`へ配置し、所有者を`root:wheel`、モードを`644`に設定してから登録します。

```sh
sudo launchctl bootstrap system /Library/LaunchDaemons/jp.tvrockgo.server.plist
sudo launchctl bootout system /Library/LaunchDaemons/jp.tvrockgo.server.plist
```

1行目が登録・起動、2行目が停止・登録解除です。launchdの出力ログ用ディレクトリは起動前に存在している必要があります。

## 処理上の違い

HTTP・予約・録画制御・DB・検索のロジックは共通です。サーバーの通常停止時は各OSともコンテキストをキャンセルし、`tvrock.Close()`で通信・バックグラウンド処理・インデックス・DBの順に停止します。

違いはUIの有無、起動・停止の通知元、実行ユーザーと権限、自動再起動の管理者です。systemd/launchdはログイン前でも起動できますが、対話画面を必要とする外部アプリには別途対応が必要です。Windowsの録画アプリや専用DLLがLinux/macOSで動くことを保証するものではありません。

OSのシャットダウンやサービス管理者の停止待機時間を超えると、OSによって強制終了されることがあります。電源断や強制終了では正常終了は保証できません。
