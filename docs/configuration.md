# 設定ガイド

導入の流れは[README](../README.md)、保存先の指定と起動オプションは[起動方式とサービス配置](startup.md)を参照してください。

| ファイル | 用途 |
| --- | --- |
| `conf/tvrock.org.yml` | 初期設定のひな形。運用設定が存在しない場合だけ読み込みます。設定ファイル`tvrock.yml`の再生成時にも必要です。 |
| `conf/tvrock.yml` | 実際に使用する設定。初回起動時にひな形から自動生成されます。 |

初回起動前に`tvrockgo.org.yml`を編集します。  
初回起動後は**サーバーを終了してから`conf/tvrock.yml`を編集し、再起動**してください。Web画面から変更できる項目は、画面上で設定できます。起動中の手動編集はサーバーによる保存と競合する場合があります。

`tvrockgo.org.yml`のひな形は地上波☓4、BS/CS☓4の計8台を想定して設定しています。

初回設定をやり直す場合は、サーバーを終了して`conf/tvrock.yml`をバックアップしたうえで削除し、ひな形を修正して起動します。設定全体が初期化されるため、運用開始後の通常の変更では削除しないでください。

以下は設定の抜粋です。既存の同名項目を編集し、同じキーを重複して追加しないでください。サンプルのパス・ドライバ名・アカウントは、実際の環境に置き換えます。

## デバイス

ひな形は地上波4台、BS/CS4台の構成です。使用しないデバイスは削除し、後述の優先順位からも取り除いてください。
```yaml
devicemacrots: &devmacrots             # 地上波マクロ定義
  apppath1: 'C:\Recorder\TVTest.exe'   # 視聴
  apppath2: 'C:\Recorder\RecTest.exe'  # 録画
  apppath3: 'C:\Recorder\TVH264.exe'   # ワンセグ視聴
  apppath4: 'C:\Recorder\RecTest.exe'  # ワンセグ録画
  recordpath: 'D:\RecordTS'            # 録画ファイルのフォルダ
  tunertype: 0                         # 0=地上波

devicemacrobscs: &devmacrobscs         # BSCSマクロ定義
  apppath1: 'C:\Recorder\TVTest.exe'   # 視聴アプリ・フルパス
  apppath2: 'C:\Recorder\RecTest.exe'  # 録画アプリ・フルパス
  recordpath: 'D:\RecordBSCS'          # 録画ファイルのフォルダ
  tunertype: 1                         # 1=BS/CS

devices:
  - <<: *devmacrots
    id: REC01                                               # デバイスID (IDで使用するのでこのフォーマットをなるべく崩さないでください)
    option1: '/DID A /reccurservice /d BonDriver_T0.dll'    # 視聴アプリ・オプション
    option2: '/DID A /reccurservice /d BonDriver_T0.dll'    # 録画アプリ・オプション
    option3: '/DID A /reccurservice /d BonDriver_T0.dll'    # 視聴アプリ・オプション（ワンセグ)
    option4: '/DID A /reccurservice /d BonDriver_T0.dll'    # 録画アプリ・オプション（ワンセグ)
  - <<: *devmacrobscs
    id: REC05                                               # デバイスID (IDで使用するのでこのフォーマットをなるべく崩さないでください)
    option1: '/DID E /reccurservice /d BonDriver_S0.dll'    # 視聴アプリ・オプション
    option2: '/DID E /reccurservice /d BonDriver_S0.dll'    # 録画アプリ・オプション
```

最新のTVTest/TSTaskの場合は以下のように設定します
```yaml
devicemacrots: &devmacrots                          # 地上波マクロ
  apppath1: 'C:\Recorder\TVTest.exe'                # 視聴アプリ・フルパス
  apppath2: 'C:\Recorder\TSTask.exe'                # 録画アプリ・フルパス
  recordpath: 'D:\RecordTS'                         # 録画ファイルのフォルダ
  tunertype: 0                                      # 0=地上波
  hybrid: true                                      # 地上波/ワンセグ兼用
devicemacrobscs: &devmacrobscs                      # BSCSマクロ
  apppath1: 'C:\Recorder\TVTest.exe'                # 視聴アプリ・フルパス
  apppath2: 'C:\Recorder\TSTask.exe'                # 録画アプリ・フルパス
  recordpath: 'D:\RecordBSCS'                       # 録画ファイルのフォルダ
  tunertype: 1                                      # 1=BS/CS
devices:
  - <<: *devmacrots
    id: REC01                                               # デバイスID (IDで使用するのでこのフォーマットをなるべく崩さないでください)
    option1: '/DID A /reccurservice /d BonDriver_T0.dll'    # 視聴アプリ・オプション
    option2: '/DID A /reccurservice /d BonDriver_T0.dll'    # 録画アプリ・オプション
  - <<: *devmacrobscs
    id: REC05                                               # デバイスID (IDで使用するのでこのフォーマットをなるべく崩さないでください)
    option1: '/DID E /reccurservice /d BonDriver_S0.dll'    # 視聴アプリ・オプション
    option2: '/DID E /reccurservice /d BonDriver_S0.dll'    # 録画アプリ・オプション
```

`id`はTvRockGO内の識別子、`/DID`は録画・視聴アプリ側の識別子です。それぞれ重複しないように設定してください。アプリによって必要な起動オプションは異なります。ワンセグを使用する場合もアプリに必要なオプションを指定します。

Windowsのパスは上の例のようにシングルクォートで囲むとバックスラッシュをそのまま記述できます。ダブルクォートを使用する場合は`"C:\\Recorder\\TVTest.exe"`のようにエスケープしてください。

地上波・ワンセグを同じアプリで扱う場合は対応するデバイスに`hybrid: true`を指定します。`tunertype`は地上波の`0`のままにし、`apppath1`・`option1`（視聴）と`apppath2`・`option2`（録画）を設定します。`tunertype`の定義は`0`（地上波）と`1`（BS/CS）です。

## 録画予約と番組情報取得の優先順位

自動録画予約の優先順位は`recordingdevice`内の`orderts`・`orderbs`・`ordercs`・`orderos`に指定します。番組情報取得は`fetchprogram`内の`deviceorder*`を使用します。

次の例は、上記の地上波1台・BS/CS1台に対応します。

```yaml
recordingdevice:
  orderts: [REC01]
  orderbs: [REC05]
  ordercs: [REC05]
  orderos: [REC01]
  assist: false

fetchprogram:
  deviceorderts: [REC01]
  deviceorderbs: [REC05]
  deviceordercs: [REC05]
  deviceorderos: [REC01]
```

複数台ある場合は使用したい順にIDを並べます。番組情報取得は録画予約の割り当てと重なりにくくするため、録画とは逆順にする方法があります。取得を行う放送種別や時刻は、WebアプリのSETTING画面で設定してください。

## 補助デバイス（任意）

同じサービスを使用する連続予約などで補助デバイスを割り当てる機能です。同一ドライバを複数の論理デバイスから使用できる環境で設定してください。初期設定では無効です。指定デバイスで固定できないタイプのドライバでは使用できないので気をつけて下さい。

補助デバイスへの割り当ては対応する通常デバイスの重複予約がすべて同じサービスで、補助デバイス自体に重複予約がない場合に行います。補助側が同じサービスで使用中なら、空いている通常側を使用します。両方が使用中、または別サービスの予約と重なる場合は、次の通常デバイスを試します。

同じサービス内で`REC01`の予約が重なると`SUB01`になり、`SUB01`の次の予約が重なると`REC01`にデバイスが割り当てられます。

すべての候補が使用中の場合は通常デバイスの中から重複時間の短いものを選ぶため録画の競合を解消できるとは限りません。


ひな形のコメント付き`SUB01`～`SUB08`を参考に補助デバイスを**既存の`devices`一覧へ追加**します。通常のデバイスと異なる`/DID`を使い、8台構成なら通常側をA～H、補助側をI～Pにします。

```yaml
# アイドル時間をスケジュールの衝突判定に含める
isafterwithmargin: true

recordingdevice:
  # orderts・orderbs・ordercs・orderosは既存の設定を残す
  assist: true
  deviceassistance:
    REC01: SUB01
    REC05: SUB05
```

対応表には実際に追加した補助デバイスだけを指定します。`registrycode`は未設定の場合だけ起動時に生成され、設定ファイルに保存されます。通常・補助デバイスとも以降は保存された値を使うため、サンプルに記述する必要はありません。

## Webサーバーとログイン

初期設定はローカルPC向けで認証が有効です。ひな形のアカウントとパスワードは必ず変更してください。

```yaml
host: "localhost:80"
allowOrigins:
  - "http://localhost"
  - "http://127.0.0.1"
allowOriginPrefix:
  - "http://192.168."

authentication: true
useraccount:
  example-user: "CHANGE-ME-before-first-start"
cookiedomain: "localhost"
sessionsecretkey: ""
sessionhttponly: true
```

パスワードは起動時にbcryptハッシュへ変換して`conf/tvrock.yml`へ保存します。空の`sessionsecretkey`にはランダムな値を生成します。生成後の設定ファイルにも認証情報が含まれるため公開しないでください。

LANの別端末からアクセスする場合は、例えば`host: "0.0.0.0:8080"`に変更し、`http://サーバーのIPアドレス:8080`へ接続します。使用するURLを`allowOrigins`に追加し、`cookiedomain`は空文字列または接続先に合う値へ変更してください。必要に応じてWindowsファイアウォールの受信規則も設定します。Originの設定は認証やファイアウォールの代わりにはなりません。

通常はローカルPCまたは信頼できるLAN内で使用してください。外部から使用する場合はHTTPSとWebSocketに対応したリバースプロキシを設置し、認証を有効にします。HTTPS運用では`sessionsecure: true`も設定してください。

## 電源管理（任意）

```yaml
powermanagersetting:
  powermanager: true
  sleeptimer: 15          # 予約のない状態などが続いてからスリープするまでの分数。0でタイマー無効
  sleeptimerpause: false  # trueで一時停止、falseで再開
```

SETTINGの「電源管理」で、予約のない時間帯のスリープや予約開始前の復帰を設定します。Windowsでは復帰用にタスクスケジューラを使用します。PCの電源設定やハードウェアにも依存するため、有効化したら短い予約でスリープ・復帰・録画を確認してください。

上の15分は設定例です。トレイのチェックと専用アイコンは一時停止していないことを示し、実際にスリープするには`sleeptimer`を0より大きく設定し、録画中でないことなどの条件を満たす必要があります。

「デバイス起動中はスリープへ移行しない」をONにすると、視聴・録画などでデバイスが起動している間はTvRockGOのスリープタイマーを停止します。「スリープを抑止」もONの場合、Windows・Linux・macOSの自動スリープ抑止も継続します。
