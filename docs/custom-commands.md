# カスタムコマンド

録画の待機開始・録画開始・録画停止に合わせて、外部プログラムやスクリプトを起動できます。番組情報はコマンドラインへ展開せず、実行ごとのJSONファイルで渡します。

## 設定

サーバーを終了してから `conf/tvrock.yml` の `customcommand` を編集し、再起動してください。初回起動前の設定は `conf/tvrock.org.yml` を使用します。運用設定の自動移行は行いません。

```yaml
customcommand:
- id: Touch
  mode: 0
  dependencies: ""
  type: batch
  apppath: "./docs/custom-command-examples/Touch.cmd"
  options: []
- id: Tweet
  mode: 1
  dependencies: "Touch"
  type: powershell
  apppath: "./docs/custom-command-examples/WriteRecordInfo.ps1"
  options:
  - "-Mode"
  - "Tweet"
  - "-OutputDirectory"
  - "C:/Temp"
```

| 項目 | 内容 |
| --- | --- |
| `id` | 予約画面などで選択するコマンドID |
| `mode` | `0`: 録画停止後、`1`: 録画開始時、`2`: 待機開始時 |
| `dependencies` | 予約で選択されたコマンドIDと一致する場合にも、このコマンドをその `mode` で実行する。別コマンドの完了・成功を待つ指定ではなく、実行順序の保証はない |
| `type` | `executable`・`batch`・`powershell`。省略時は `executable` |
| `apppath` | 固定の実行ファイルまたはスクリプト。相対パスは保存先ディレクトリ（`--data-dir`）を基準に解決する |
| `options` | 固定引数の配列。配列の1要素を1引数として渡す。番組情報の置換トークンは使用不可 |

上の例では、予約で `Touch` を選択すると、録画開始時に `Tweet`、録画停止後に `Touch` を実行します。予約の設定・実行状況により、録画パスなどが未確定のタイミングもあります。

## 実行タイプ

| `type` | 実行方法 |
| --- | --- |
| `executable` | シェルを挟まず直接起動。`.bat`・`.cmd`・`.ps1` は指定不可。ファイル名だけの場合はPATHから検索する |
| `batch` | Windows専用。`.bat`・`.cmd` をシステムの `cmd.exe /d /v:off /s /c` から起動 |
| `powershell` | `.ps1` を `-NoLogo -NoProfile -NonInteractive -File` で起動。WindowsはシステムのWindows PowerShell、Linux/macOSはPATH上の `pwsh` を使用 |

スクリプトの拡張子は大文字小文字を区別せず検査します。作業ディレクトリは実行対象のファイルがあるフォルダーです。`executable` でPATH検索する場合は保存先ディレクトリを使用します。

`batch` のパスには `"`・`%`・`!`・改行を使用できません。固定引数にも `"`・`%`・`!`・`&`・`|`・`<`・`>`・`^`・改行を使用できません。空白を含むパス・引数はTvRockGOが引用するため、設定値に追加の引用符を含めないでください。これらの制限に該当する値や番組情報は、JSONから処理してください。

Windowsではコンソールウィンドウを非表示にし、標準出力・標準エラー・終了コードを取得します。PowerShellの実行ポリシーを自動で変更・回避しません。実行ポリシーで拒否された場合は、管理方針に沿ってスクリプトの署名・許可を設定してください。

## JSONの受け取り

子プロセスの環境変数 `TVROCK_CONTEXT_FILE` に、その実行専用JSONファイルの絶対パスを設定します。親プロセスに同名の環境変数があっても置き換えます。

PowerShellでは次のように読み取れます。

```powershell
$info = Get-Content -LiteralPath $env:TVROCK_CONTEXT_FILE -Raw -Encoding UTF8 |
    ConvertFrom-Json
$title = $info.title
```

バッチから、同じフォルダーの固定スクリプトを呼ぶ例です。環境変数はPowerShellにも継承されます。

```bat
@echo off
setlocal DisableDelayedExpansion
"%SystemRoot%\System32\WindowsPowerShell\v1.0\powershell.exe" -NoLogo -NoProfile -NonInteractive -File "%~dp0WriteRecordInfo.ps1" -Mode Touch
exit /b %errorlevel%
```

JSONはUTF-8で、番組名・引用符・改行などをデータとして保持します。実行ごとにOSの一時ディレクトリ内へ専用フォルダーを作成し、終了・起動失敗・キャンセル時に削除します。Unixではフォルダー0700、ファイル0600で作成します。Windowsのアクセス権は一時フォルダーの継承ACLに従います。

JSONはコマンド終了までに読み取ってください。`start /b` などで処理を切り離して先に終了すると、後続プロセスが読む前に削除されます。親プロセスを強制終了した場合などは一時ファイルが残る可能性があります。

## 置換トークンとの対応

従来のトークンは、カスタムコマンドの `apppath` と `options` ではすべて禁止します。実行直前の検証でエラーとなり、そのコマンドは起動しません。録画ファイル名の `recordersetting.filenameex` の置換機能は従来どおりです。

`start.hour` などは入れ子のJSONキーです。日付・時刻の各項目は文字列、`durationMinutes` と `errorCount` は数値です。

| 従来のトークン | JSONキー | 内容 |
| --- | --- | --- |
| `@TT` | `title` | 予約タイトル。空の場合は番組DBのタイトル |
| `@SB` | `subtitle` | 先頭の空白は含まない。従来の空白が必要なら出力側で追加 |
| `@sb` | `subtitle` | 番組DBのサブタイトル |
| `@CH` | `stationName` | 局名 |
| `@DR` | `durationMinutes` | 番組DBの終了－開始を分に換算した整数 |
| `@HTK` | `keywordHashtags` | `#タグ1 #タグ2` 形式。未設定は空文字列(ハッシュタグ定義時に","で複数定義できます) |
| `@HTS` | `stationHashtags` | 同形式の局のタグ。未設定は空文字列(ハッシュタグ定義時に","で複数定義できます) |
| `@EC` | `errorCount` | 録画エラー数。従来の最低2桁表示は出力側で整形 |
| `@RD` | `recordDirectory` | 録画フォルダー（末尾の区切り文字を含む） |
| `@RF` | `recordFile` | 録画ファイルパス |
| `@RN` | `recordFileName` | 録画ファイル名 |
| `@YY` | `current.year` | コマンド実行時の年 |
| `@2YY` | `current.yearShort` | 同・年の下2桁 |
| `@MM` | `current.month` | 同・月 |
| `@DD` | `current.day` | 同・日 |
| `@yy` | `current28.year` | 実行時の28時間表記の日付・年 |
| `@2yy` | `current28.yearShort` | 同・年の下2桁 |
| `@mm` | `current28.month` | 同・月 |
| `@dd` | `current28.day` | 同・日 |
| `@SH` | `start.hour` | 番組開始の時（00～23） |
| `@SM` | `start.minute` | 同・分 |
| `@SS` | `start.second` | 同・秒 |
| `@WK` | `start.weekday` | 同・曜日（`月` など） |
| `@WE` | `start.weekdayEnglish` | 同・英語曜日（`Monday` など） |
| `@EH` | `end.hour` | 番組終了の時（00～23） |
| `@EM` | `end.minute` | 同・分 |
| `@ES` | `end.second` | 同・秒 |
| `@sh` | `start28.hour` | 番組開始の時（28時間表記） |
| `@sm` | `start28.minute` | 同・分 |
| `@ss` | `start28.second` | 同・秒 |
| `@wk` | `start28.weekday` | 同・28時間表記の日付の曜日 |
| `@we` | `start28.weekdayEnglish` | 同・英語曜日 |
| `@eh` | `end28.hour` | 番組終了の時（28時間表記） |
| `@em` | `end28.minute` | 同・分 |
| `@es` | `end28.second` | 同・秒 |

日時オブジェクト `current`・`current28`・`start`・`start28`・`end`・`end28` はすべて `year`・`yearShort`・`month`・`day`・`hour`・`minute`・`second`・`weekday`・`weekdayEnglish` を持ちます。月・日・年下2桁は従来どおりゼロ埋めせず、時・分・秒は2桁です。28時間表記は午前0～3時を前日の24～27時として扱います。

便利項目として次のキーも含めます。

| キー | 内容 |
| --- | --- |
| `date` | コマンド実行日 `yyyy/MM/dd`（こちらは月・日をゼロ埋め） |
| `startTime` | 番組開始 `HH:mm` |
| `startSeconds` | 番組開始の秒 `ss` |
| `endTime` | 番組終了 `HH:mm` |

現在日時はTvRockGOの放送時刻補正を含み、日付・時刻の表示にはサーバーのローカルタイムゾーンを使用します。現在日時は番組開始日とは限りません。対応する番組DBの情報がない場合、開始・終了の各文字列とサブタイトルは空、放送時間は0です。録画パス未確定時はパス関連の文字列も空です。番組の内容・説明文は今回のJSONには含めません。

## Touch・Tweet・Directoryのサンプル

[custom-command-examples](custom-command-examples/) に以下を同梱しています。

| ファイル | 処理 |
| --- | --- |
| `Touch.cmd` | 固定の `WriteRecordInfo.ps1 -Mode Touch` を呼ぶ |
| `Tweet.cmd` | 固定の `WriteRecordInfo.ps1 -Mode Tweet` を呼ぶ |
| `WriteRecordInfo.ps1` | Touchは `record.txt`、Tweetは `tweet.txt` にUTF-8で追記する。テキストに追記するだけでSNS投稿は行いません。 |
| `Directory.ps1` | 録画パスがある場合、対象ファイルの情報を取得する |

追記先は既定で `C:\Temp` です。事前に作成して書き込み権限を確認してください。PowerShellタイプで直接呼ぶ場合は固定引数 `-OutputDirectory` で変更でき、バッチの場合はバッチ内の固定呼び出しへ追加できます。既存の出力ファイルを使う場合は文字コードをUTF-8に合わせてください。ひな形の旧Touchの時刻 `@SH:@SH@SS` は、サンプルでは `HH:mm:ss` に整理しています。新しいファイルは作成できますが、出力先フォルダーは自動作成しません。

Linux/macOSでは `batch` の代わりに `powershell` と `pwsh` を使い、出力先を環境に合わせて変更してください。

ストレージ監視用に使用している`storagecommand` も同じタイプとJSON受け渡しに対応します。ひな形のように固定のPowerShellコマンドだけを使う設定は、タイプ省略時の `executable` として従来どおり実行できます。番組情報のないストレージ調査では番組関連フィールドは空または0です。

JSON値を `Invoke-Expression`、`cmd /c`、`CALL` などに渡してコードとして再評価しないでください。実行ファイルやスクリプト自体、固定引数は信頼できる管理者が設定する前提です。JSON化はスクリプトを隔離する機能ではなく、スクリプトはTvRockGOの実行ユーザーの権限で動作します。
