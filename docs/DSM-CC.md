# DSM-CC / BML対応

TvRockGOのデータ放送・局ロゴ受信処理をまとめた技術資料です。導入は[README.md](../README.md)、開発環境とビルドは[AGENTS.md](../AGENTS.md)を参照してください。

## 構成

| ソース | 役割 |
| --- | --- |
| 外部のtvrockdtvプロジェクト（`src/dsm.rs`、`src/engine.rs`） | PAT/PMTからDSM-CC PIDを特定し、セクション・イベント・PCR等を`DSM:` JSONとして送信。このリポジトリには含まれません。 |
| `tvrock/deviceserver.go` | `DSM:`の受信。復元状態をデバイス接続ごとに保持し、選局時に破棄。 |
| `tvrock/dsm.go`、`dsm_manifest.go`、`dsm_events.go` | DII/DDB、バージョン更新、zlib圧縮、ARIB MIME multipart、放送イベント・PCRの処理。 |
| `tvrock/bml.go` | BMLの静的HTML変換。UTF-8・Shift_JIS・EUC-JPに対応し、静的表示用HTMLからスクリプトとイベント属性を除去。 |
| `app/src/components/BMLWindow.vue` | BML表示用ソースを保持。再生ライブラリを使う処理はコメント化して無効化。 |
| `app/src/components/DeviceController.vue`、`app/src/App.vue` | WebSocket受信、選局・切断・PMT変更時の受信状態の消去。データ放送の表示UIはコメント化して無効化。 |

## 局ロゴ

`LOGO-00`～`LOGO-05`および`CS_LOGO-00`～`CS_LOGO-05`のモジュールから対象サービス一覧と画像を解析します。既存の`receiveCDT` / `saveCDT`を利用し、実行ファイルの配置先にある`static/cdt/<内部Sid>_<LogoType>`へ保存します。BS/CS混在モジュールはONIDで振り分けます。

Webアプリのビルドは`static/`をクリーンするため、受信済みの局ロゴを保存する場合は事前にバックアップするか、別出力先へビルドしてください。

## 通信と再接続

送信側は完全なDSM-CCセクションを16進文字列として送ります。形式の例（`Data`は省略表記）:

```text
DSM:{"Sid":101,"Tsid":1,"Onid":4,"Pid":288,"Components":[{"Pid":288,"ComponentTag":64,"DataComponentId":7,"Info":"3340f880"}],"Data":"3b..."}
```

`Kind:"pmt"`はコンポーネント一覧、`PCRBase` / `PCRExtension`は再生時刻を表します。DDBのsection_numberやcurrent_next_indicatorは通常のPSI制約と異なる扱いが必要です。

サーバーは全ブロックが揃ったモジュールを`bml:`としてブラウザへ通知します。受信中は最新スナップショットを15秒ごとにも送信し、後から開いた画面や再接続したWebSocketへ反映します。Webアプリには受信状態を保持する処理が残っていますが、表示・再生は行いません。

## 起動と更新

1. サーバーと録画・視聴アプリを終了し、設定・DBをバックアップします。
2. 公開リポジトリの[tvrockdtv](https://github.com/tvrock-master/tvrockdtv)プロジェクトで用意する対応`TvRockDTV.dll`を、録画・視聴アプリと同じビット数で配置します。
3. このリポジトリの手順でビルドしたサーバーとWebアプリを配置し、サーバーを起動します。
4. ブラウザを再読み込みします。局ロゴ受信は利用できますが、データ放送の表示・再生UIはありません。

## 検証

リポジトリルートから実行します。テスト用の一時データを使用し、運用中の設定・DBや録画アプリは使用しません。

```powershell
go test ./tvrock -run 'TestDSM|TestCDT' -count=1
go test ./...
```

テストは受信・HTML通知、DII/DDB、順不同・重複・欠落、CRC、zlib、文字コード、局ロゴ、受信機向けダウンロードの除外などを確認します。合成データによるテストは、実放送の全局での動作確認を代替しません。

## 対応範囲

国内ARIBデータカルーセルのMIMEモジュールを対象とします。汎用BIOPオブジェクトカルーセルのファイルシステム復元は実装していません。モジュールは圧縮前後それぞれ2MiBまで、デバイスごとの宣言サイズ合計は16MiBまでに制限します。

ブラウザ側のBML再生ライブラリの依存は削除済みです。表示・リモコン操作UIのソースは保持し、表示と再生ライブラリを使う処理はコメント化して無効化しています。サーバー側のDSM-CC受信、静的HTML変換、WebSocket通知は残しています。

PMTでBMLと識別されたコンポーネントをMIME解析対象とし、局ロゴは既存の識別・CDT保存経路を利用します。受信機向けダウンロード（`DataComponentId=9`）は、全受信機共通データの`component_tag=0x79`・`0x7A`に限りDII/DDBを解析し、`LOGO-00`～`LOGO-05`・`CS_LOGO-00`～`CS_LOGO-05`だけを組み立てて保存します。その他のモジュールは本文を保持せず、BMLとしての解析・通知やファームウェアの保存・実行は行いません。他のコンポーネントとイベントセクションは、メタデータ・PID・セクション長・CRC・テーブルIDの検証後に処理を終了します。

## 参照

- [LibISDB LogoDownloaderFilter](https://github.com/DBCTRADO/LibISDB/blob/master/LibISDB/Filters/LogoDownloaderFilter.cpp)
