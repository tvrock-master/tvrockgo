# マニュアル画像の更新

このフォルダは[Webアプリのマニュアル](../../tvguide.md)と[Rockbarのマニュアル](../../rockbar.md)で使用する画像です。放送局名、番組名、本文、字幕はすべて撮影スクリプト内の架空データです。実放送のデータや局ロゴは読み込みません。オレンジの枠は説明対象を示す注釈です。

## 再撮影

通常のapp・rockbarのnpm依存に加え、別途PlaywrightとMicrosoft Edgeを用意します。リポジトリのルートで実行してください。

```powershell
npm run build --prefix app -- --dest ../.tmp/manual-preview
node scripts/generate-manual-images.cjs
```

Playwrightが通常のモジュール探索先にない場合は、`PLAYWRIGHT_PATH`にインストールしたモジュールのディレクトリを指定します。ローカルのMaterial Design Iconsを使う場合は、`MDI_CSS`にCSSファイルのパスを指定し、同じディレクトリに`materialdesignicons-webfont.woff2`を用意してください。画像と一緒にフォントを再配布する処理は行いません。

`MANUAL_TARGET`を`web`または`rockbar`にすると片方だけ再撮影できます。未指定では両方を更新します。

撮影スクリプトはループバックの空きポートでサンプル専用HTTP/WebSocketサーバーを起動し、処理後に終了します。運用サーバー、設定、DB、録画アプリを利用しません。ブラウザは一時プロファイルで起動し、外部のフォントなどへのアクセスも遮断します。Rockbarのネイティブ操作はサンプル用の応答に置き換えており、TVTestの実起動や実録画を検証する画像ではありません。

番組表のビルド出力は`.tmp/manual-preview`に限定してください。運用中の`static/`へのビルドは局ロゴなどを消す場合があります。スクリーンショットは本フォルダの同名PNGを上書きします。
