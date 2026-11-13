# TvRockGO Webアプリ

Vue 2・Vuetify 2で実装した番組表、録画予約、検索、設定画面です。

TvRockGOサーバーから配信して使用します。サーバーの導入・録画環境の準備は[ルートのREADME](../README.md)、プロジェクト全体の開発方針は[AGENTS.md](../AGENTS.md)を参照してください。

## 起動と接続

TvRockGOを起動し、初期設定なら[http://localhost](http://localhost)を開いてログインします。別のPCやポートを使う場合はサーバーのアドレスに置き換えてください。ルートURLから`/static/index.html`へ移動します。HTMLファイルを直接開いて使用する構成ではありません。

HTTP APIはページと同じorigin、WebSocketは同じホストとポートの`/con`へ接続します。HTTPSで開いた場合は`wss:`を使います。RockbarのElectron版のような接続先URLの設定はなく、ブラウザで開いたURLが接続先になります。

局・番組が表示されない場合は、[局情報と番組情報の取得](../README.md#局情報と番組情報の取得)に従い、デバイスの起動、サービスの選択、番組情報取得局・時刻の設定を行ってください。

## 画面と操作

上部のタブで画面を切り替えます。

| タブ | 内容 |
| --- | --- |
| TimeTable | 地上波・BS・CS・ワンセグの番組表、番組詳細、録画・視聴予約 |
| Schedule | 予約一覧、有効・無効の切り替え、予約内容の編集・削除、実行中予約の停止 |
| Search | タイトル・本文による番組検索、過去番組・字幕検索、検索予約の登録 |
| Keyword | キーワード予約の一覧、条件・録画設定の編集、有効・無効の切り替え・削除 |
| Device | デバイスの起動・終了、選局、録画、キャプチャ、字幕・受信状態の確認 |
| Setting | 表示、サービス、番組情報取得、録画・電源管理の設定、ログ、ログアウト |

右上の縦三点ボタンでサイドナビゲーションを開閉します。番組表の放送波・時間帯の変更や、番組・予約の確認に使えます。

### 番組表

放送波・日付・時間帯を選び、局ごとの番組を表示します。週間表示にも切り替えられます。局名行はスクロール中も固定されます。

番組欄の詳細表示は、Setting →「番組表」の「番組欄表示アクション」でクリック・マウスオーバー・ポップアップから選びます。「番組クリック時に字幕履歴を表示」が有効な場合、同じ番組欄のクリックは詳細表示 → 字幕履歴 → 閉じるの順に切り替わります。字幕履歴はサーバーに保存された字幕と受信中の更新を表示します。

番組欄の予約ボタンから録画予約を登録します。「視聴予約ボタンを表示」を有効にすると視聴予約も操作できます。予約後のデバイス、待機時間、開始・終了余裕時間、番組時間追従、終了処理、カスタムコマンド等はScheduleで確認・編集します。カスタムコマンドの設定方法は[専用資料](../docs/custom-commands.md)を参照してください。

### 番組・字幕検索とキーワード予約

Searchの`input title`と`input content`に条件を入力し、局名・ジャンル・AND・あいまい検索を組み合わせます。「過去番組検索」では日付範囲を指定でき、終了日を含めて検索します。日付が未指定の場合は全期間が対象です。「字幕」を使うと保存済み字幕を検索できます。

検索結果から番組を選択して予約できます。今後の番組の検索条件は「検索予約登録」でキーワード予約として保存し、Keywordから条件や録画設定を変更できます。検索日数・時間帯・デバイス・録画余裕時間・視聴のみ等も指定できます。

### デバイス・字幕

Deviceでは、停止中のデバイスを「視聴」「録画」から起動し、起動後にサービスを選択します。「録画」でのアプリ起動と「録画開始」は別操作です。起動状態に応じて録画開始・停止、キャプチャ、終了等の操作を表示します。実際の録画・視聴・キャプチャには、設定済みの対応アプリと専用DLL等が必要です。

字幕スイッチでデバイスごとの字幕表示を切り替えます。字幕の再生タイミングは`captionPlayback.js`で処理します。番組表から開く字幕履歴は`CaptionHistoryWindow.vue`、Deviceの字幕表示は`CaptionWindow.vue`が担当します。

デバイスは個別に折りたためます。Setting →「デバイス」では字幕表示の初期値と、電源OFF時の自動折りたたみを設定できます。

## 表示・サーバー設定

Setting →「番組表」で次の表示を変更できます。

- カラーテーマ：「Windowsに合わせる」「ライト」「ダーク」。自動設定ではブラウザが通知するOSの配色に従い、起動中の変更にも追従します。
- サービス表示範囲：2～12列。サブタイトル・番組詳細の表示閾値、時間軸の配置間隔も指定できます。
- 局ロゴ、28時間表記、放送波・時間帯の横配置、番組クリック時の字幕履歴、視聴予約ボタンの表示。

表示設定やデバイスの折りたたみ状態はブラウザの`tvrockgo_options` Cookieに保存します。有効期間は保存から1年で、Rockbarのローカルストレージとは独立しています。

サービスの表示対象、番組情報取得局・時刻、録画設定、電源管理等はサーバー側の設定です。Settingにはストレージ情報、録画ログ、システムログ、ログアウトもあります。設定項目の意味とYAML設定は[設定ガイド](../docs/configuration.md)を参照してください。

電源管理が有効な場合はSettingに「電源管理」を表示し、上部のアプリロゴのクリックでもスリープタイマーを切り替えられます。タイマーが有効なときはスリープ用ロゴになります。

## 開発・ビルド

CIと同じNode.js 22を使用し、リポジトリの`app/`で実行します。依存は`package-lock.json`を基準にインストールします。

```powershell
npm ci
npm run serve
npm run build
```

| コマンド | 内容 |
| --- | --- |
| `npm run serve` | Vue CLIの開発サーバー。コード変更を自動反映 |
| `npm run build` | 配信用ファイルを`../static/`へ出力 |
| `npm run lint` | Vue CLIのlintを実行。既定で自動修正を伴うため差分を確認 |

**通常のビルドは`static/`をクリーンするため、受信済みの`static/cdt/`の局ロゴと`static/rockbar/`も削除します。** 運用フォルダでは事前にバックアップするか、次のように別出力先へビルドしてください。

```powershell
npm run build -- --dest ../.tmp/frontend-preview
```

WebアプリとRockbarのブラウザ版を両方更新する場合は、リポジトリルートから次の順で実行します。

```powershell
npm run build --prefix app
npm run build:web --prefix rockbar
```

`npm run serve`にはGo APIへのプロキシ設定がありません。API・WebSocketの接続先も開発サーバー自身になるため、通信を伴う確認にはGoサーバー配信のビルドを使用するか、開発用プロキシを構成してください。検証のために運用設定・DBでサーバーを起動しないでください。

バージョンは`vue.config.js`で`APP_VERSION`、`VUE_APP_VERSION`、`CI_COMMIT_TAG`、GitHubのタグ名の順に選び、先頭の`v`を除いてビルド時に埋め込みます。未指定時は`dev`です。配布物全体の作成は[リリース手順](../docs/releases.md)を参照してください。

## テスト

通常のフロントエンドテストは、`app/`で次のように実行できます。`package.json`に`npm test`は定義していません。

```powershell
node --test tests/captionHistory.test.cjs tests/captionPlayback.test.cjs tests/captionWindow.test.cjs tests/frontendReview.test.cjs tests/clientIdentity.test.cjs tests/deviceReconnect.test.cjs tests/systemTheme.test.cjs
```

字幕履歴・再生・表示、画面の回帰検査、クライアント識別値、デバイス再接続、テーマ追従を確認します。

次の2件はPlaywrightとMicrosoft Edge（`msedge`チャネル）が必要です。Playwrightは通常のnpm依存に含まれません。各テストの内容と追加環境を確認してから実行してください。テスト内の画面・モックを使い、稼働中の録画サーバーへは接続しません。

```powershell
node tests/captionHistoryLifecycle.test.cjs
node tests/timeTableBrowser.test.cjs
```

フロントエンド変更時は、別出力先へのビルドも確認します。実機での録画・専用DLLとの接続・スリープ復帰は、これらのテストでは確認できません。

## 通信・実装

Vue 2・Vuetify 2、Vue CLI、axiosを使用します。主なソースの役割は次のとおりです。

| パス | 役割 |
| --- | --- |
| `src/main.js` / `src/App.vue` | Vue起動、HTTP接続先、共通状態、WebSocket、通知・再接続、表示設定の保存 |
| `src/components/TimeTable.vue` / `src/timeTableRenderer.js` | 番組表の操作・描画、番組詳細・予約操作 |
| `src/components/RecordingSchedule.vue` | 予約一覧・編集・停止 |
| `src/components/ProgramSearch.vue` / `SearchResults.vue` / `KeywordReserve.vue` | 検索条件、結果、キーワード予約 |
| `src/components/DeviceController.vue` | デバイス操作、受信情報、字幕・データ放送 |
| `src/captionPlayback.js` / `src/components/Caption*.vue` | 字幕の再生・表示・履歴 |
| `src/components/SystemSetting.vue` / `src/components/setting/` | 各設定画面 |
| `src/systemTheme.js` / `src/plugins/vuetify.js` / `src/assets/theme.css` | テーマ追従・配色 |
| `src/clientIdentity.js` / `src/define.js` / `src/utility.js` | クライアント識別、定数、共通処理 |
| `public/` / `vue.config.js` | HTML・manifest・アイコン、ビルド設定 |

主なHTTP APIは次のとおりです。

| パス | 内容 |
| --- | --- |
| `/cnf` / `/stl` / `/dvs` | 共通設定、局、デバイス |
| `/tvp` / `/tvf` | 番組表、注目番組 |
| `/sch` / `/rst` | 予約、実行状態 |
| `/src` / `/kwr` / `/caption` | 検索、キーワード予約、字幕履歴 |
| `/fps` / `/rcs` / `/pms` / `/log` | 番組情報取得時刻、録画設定、電源管理、ログ |

`/con`のWebSocketでデバイス状態（`dev:`）、番組・局・予約・設定の更新通知、検索結果、字幕（`cap:`）、BML（`bml:`）等を受信します。デバイス操作は`BDV:` / `COM:` / `REQ:`を使います。切断時は自動再接続し、試行間隔は最大30秒です。再接続後は`REL:`でデバイス状態を再取得します。

クライアント識別値は`TvRockApp/<バージョン>`です。HTTP APIでは`X-TvRock-Client`ヘッダ、WebSocketでは同名のクエリパラメータに付けます。サーバーの識別値検証に対応したビルドを使用してください。ログイン・セッションとWebSocketのOrigin検査はサーバー側で扱います。

`src/define.js`の`MPEG2TS`は既定で`false`です。通常画面には実験用のTestプレイヤータブを表示しません。

## manifest・アイコン・外部アセット

`public/manifest.json`でアプリ名、起動URL（`/`）、表示モード（`standalone`）を指定します。manifestのアイコンは`public/image/tvrockgo192x192.png`と`public/image/tvrockgo.png`、Apple用は`public/image/tvrockgo180x180.png`、faviconは`public/favicon.ico`です。変更はビルド出力の`static/`ではなく`public/`で行ってください。

上部ロゴは`src/assets/tvrockgo.png`と`src/assets/tvrockgo_sleep.png`を使用します。Windowsトレイも一部のPNGを埋め込むため、画像の変更時は[開発ガイド](../AGENTS.md#トレイ電源管理を変更するとき)を確認してください。

フォントはGoogle FontsのRoboto・Noto Sans JP・Noto Sans Symbols等、アイコンフォントはjsDelivrのMaterial Design Iconsを読み込みます。サーバーとの通信に加え、これらの外部アセットへのアクセスも表示に関係します。

## 関連資料

- [導入・運用](../README.md)
- [設定ガイド](../docs/configuration.md)
- [Rockbarの操作・開発](../rockbar/README.md)
- [リモート視聴](../docs/remote-viewing.md)
- [開発ガイド](../AGENTS.md)
- [リリース手順](../docs/releases.md)
