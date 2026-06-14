# AGENTS.md

このファイルは、本プロジェクト（Markmiru）で作業する際のガイダンスです。

## プロジェクト規約

- **やりとりはすべて日本語で行う。**
- **生成する `.md` ファイルもすべて日本語で保存する。**（応答文・コメント・ドキュメント・Markdown ファイルなど、出力するテキストは日本語）
- 不明な点は推測で進めず、必ず確認する。仕様・方針・選択肢の選定はユーザーに質問する。
- **コミットメッセージは英語、1〜2行。箇条書きでの更新内容の説明は不要。**`Co-Authored-By:` や `Generated with` などの自動付記は不要。
- **リポジトリに含めるのは「これがなければ最終生成物を作れない」ソースのみ。** npm パッケージ・Go モジュールキャッシュなどの外部取得物、ビルド成果物（`.exe` 等）、中間生成物（`frontend/dist` 等）は含めない。ただし自動生成されたプレースホルダー（アイコン等、後で独自に差し替える前提のもの）は例外的に含める。
- **最終生成物（`build/bin/` 配下の実行体）に影響する変更を行った場合は、必ず正式ビルドスクリプトでビルドが通ることを確認する。** フロントエンドのみの変更であっても例外なく実施する。**動作確認・検証も必ずこの正式ビルドで生成した成果物に対して行う**（素のバイナリで確認しない）。この規約は **Windows / macOS / Linux すべてに適用**する。
  - ビルドコマンド: **Windows** = `& .\scripts\build.ps1` / **macOS・Linux** = `./scripts/build.sh`（または `bash ./scripts/build.sh`）。いずれも git ショート SHA をバージョンとして埋め込むラッパーで、内部で `wails build` を実行し、**成果物は必ず `build/bin/` に出力される**（プロジェクト直下には作らない）。
  - 素の `wails build` でも生成自体は可能だが、その場合バージョンは `dev` になる。
  - **引数なしの `go build` は使わない**（Go の仕様でプロジェクト直下に実行体を生成し作業ツリーを汚すため）。コンパイルが通るかの確認だけなら成果物を残さない `go build ./...` を用いる。実行可能なバイナリが必要な場合も出力先は必ず `build/bin/` 配下にする（`go build -o build/bin/Markmiru .`）。

## アプリ概要

Markdown ドキュメントの**閲覧・編集**を行うデスクトップアプリ「**Markmiru**」（Markdown + 見る）。

- 綺麗にレンダリングして表示する「**閲覧モード**」と、Markdown 表記を直接編集する「**編集モード**」を切り替えて使う
- どちらかといえば**閲覧に重点**を置く
- セッション復元時（前回開いていたファイルを再度開く場合）は**常に閲覧モード**で開く（終了時のモードは保存しない）
- 信頼できない Markdown を開いても安全なよう防御を行う（HTML サニタイズ・CSP・外部リンクは確認ダイアログ後に OS ブラウザへ委譲・外部画像はファイルごとに表示確認）
- 画像はローカルパス（相対・絶対・ルート相対）を data URI 化して表示し、外部（リモート）画像はファイルごとに表示可否を確認する
- 多重起動は防止し、2 つ目の起動は既存ウィンドウにファイルを渡して前面化する（単一インスタンス）

## 要件と優先度

| 区分 | 要件 | 状態 |
|------|------|------|
| MUST | Markdown を綺麗にレンダリングして閲覧できる | ✅ 実装済み |
| MUST | mermaid を利用した図表もレンダリングして閲覧できる | ✅ 実装済み |
| MUST | プラグインなしの単体アプリとして動作する | ✅ 実装済み |
| MUST | 起動が素早い | ✅ 実装済み |
| MUST | マルチタブで複数ドキュメントを切り替えて表示できる | ✅ 実装済み |
| SHOULD | スタイルの修正/変更ができる | ✅ 実装済み |
| SHOULD | PDF 出力できる | ✅ 実装済み |
| SHOULD | クロスプラットフォーム対応 | 🔄 コード対応済み（Windows のみ動作確認済み） |
| SHOULD | Markdown の編集ができる（編集モードでの編集） | ✅ 実装済み |
| SHOULD | 「閲覧モード」と「編集モード」を切り替えられる | ✅ 実装済み |
| SHOULD | 不正な Markdown への防御（CSP・外部リンク制御・外部画像の表示確認・DOMPurify） | ✅ 実装済み |
| SHOULD | ローカル画像の表示（相対・絶対・ルート相対パス、data URI 化） | ✅ 実装済み |
| SHOULD | 多重起動の防止（単一インスタンス、2 つ目の起動は既存へファイル受け渡し） | ✅ 実装済み |

## 対象プラットフォーム

- **初回対象: Windows**
- **最終目標: Windows / macOS / Linux（デスクトップ3種）**
- モバイル（iPhone / Android）は**対象外**（Wails 採用に伴いモバイル不可。この前提で確定）
- 配布形態: 各プラットフォームに適した形式（例: Windows はインストーラ形式）

## 技術スタック

| レイヤー | 採用技術 | 備考 |
|----------|----------|------|
| 実装基盤 | **Wails v2**（安定版） | Go バックエンド ＋ OS 標準 WebView。v3 はアルファのため不採用 |
| バックエンド言語 | **Go** | ファイル I/O、タブ/ウィンドウ管理、ネイティブメニュー、PDF 出力起動など（薄い層） |
| WebView | OS 標準 | Win=WebView2 / macOS=WKWebView / Linux=WebKitGTK |
| フロントUI | **Svelte + Vite + TypeScript** | コンパイル方式で軽量・高速 |
| Markdown 解析 | **markdown-it** | プラグインによる拡張が豊富 |
| 図表 | **mermaid.js** | WebView 内でそのまま描画。`securityLevel: 'strict'` |
| コードハイライト | **highlight.js** | |
| 編集モード | **CodeMirror 6** | 行番号＋簡易ハイライト。軽量 |
| サニタイズ | **DOMPurify** | レンダリング HTML の無害化（script 等除去・外部リンクへ `rel` 付与・外部画像の遮断制御） |
| セキュリティ | **CSP**（本番ビルドのみ注入） | `script-src 'self'` 等。外部リンクはクリック時に確認ダイアログを挟み `BrowserOpenURL` で OS ブラウザへ委譲 |
| PDF 出力 | WebView の印刷 → PDF | 専用ライブラリ不要 |
| スタイル変更 | CSS（テーマ切替） | |
| 同梱フォント | **Noto Sans/Serif/Mono JP**（@fontsource） | 本文ゴシック/明朝・等幅。自己ホスト |
| マルチタブ | フロントエンド側で実装 | 1ウィンドウ内のタブバーで管理 |

## 選定理由（要約）

- **Wails**: mermaid を WebView でそのまま描画でき（MUST）、Electron と違い OS 標準 WebView を使うため起動が速く軽量（MUST）。Win/mac/Linux 対応で Go で書ける。安定版の v2 を採用。
- **不採用**: Electron（起動が重い）、Tauri v2（同等候補だが Go の好みを優先）、Fyne/Gio（mermaid 描画困難）、Flutter/.NET MAUI（mermaid・Linux 対応で不利）。

詳細は `docs/技術選定.md` を参照。

## 残タスク

- ライトモード編集画面のシンタックスハイライト改善
- 印刷時のコード・mermaid の配色改善
- 配布パッケージ作成（Windows: インストーラ形式; macOS/Linux: 将来対応）

## 埋め込みドキュメント（ヘルプメニュー）

- **ライセンス**: Markmiru 自体＋サードパーティのライセンスを `LICENSE.md`（リポジトリ直下）に統合。`//go:embed` で実行バイナリに埋め込んで同梱する（別ファイルの配置は不要）。ネイティブメニュー「ヘルプ → ライセンス...」で編集不可タブとして表示する（`ReadLicense()` → `openLicense()`）。
- **About（README）**: `README.md`（リポジトリ直下）も `//go:embed` で埋め込み、ネイティブメニュー「ヘルプ → Markmiru について...」で編集不可タブとして表示する（`ReadReadme()` → `openReadme()`）。専用 About 画面は設けず、README をその代替とする。
- いずれも `filePath=null` の readOnly タブで開き、「開いているファイル一覧」やセッションには残さない。

## アプリアイコン

- **Windows**: `build/windows/icon.ico` を差し替えればそのまま埋め込まれる（Wails は既存なら再生成しない。無い場合のみ `build/appicon.png` から生成）。
- **macOS**: Wails は `.icns` を**常に `build/appicon.png` から生成**し、既製 `.icns` を読み込む口がない。そのため手作りの `build/darwin/iconfile.icns` を**ビルド後フックで .app バンドルへ上書きコピー**する方式を採用（`wails.json` の `postBuildHooks` → `darwin/*`）。フックは作業ディレクトリ `build/bin`・シェル非経由で実行されるため、コマンドは `cp ../darwin/iconfile.icns Markmiru.app/Contents/Resources/iconfile.icns`。非ネイティブ（Windows 上での darwin 指定等）では自動スキップされる。

## バージョン番号

- **版＝git のショート SHA**（作業ツリーが汚れている＝`git status --porcelain` が非空なら `-dirty` を付与）。
- **埋め込み方法（ビルドスクリプトが両方を実施）**:
  - **OS プロパティ**: `wails.json` の `info.productVersion` に SHA を一時注入 → Wails が `build/windows/info.json`（Windows）・`build/darwin/Info.plist`（macOS）へ展開して実行バイナリに埋め込む。注入後は `wails.json` を**元に戻す**（リポジトリに SHA 差分を残さない。`info.productVersion` の既定は `dev`）。Windows「詳細 → 製品バージョン」に表示（`File version` は数値固定 `0.0.0.0`＝`build/windows/info.json` の `fixed.file_version`）。
  - **実行時表示**: Go の ldflags `-X main.version=<sha>`（`main.version` の既定は `dev`）。`ReadReadme()` が About（README）タブ先頭に「バージョン: <sha>」を追記。
- **ビルドスクリプト**: `scripts/build.ps1`（Windows・動作確認済み）。`scripts/build.sh`（macOS/Linux 用、同等処理）は **macOS で動作確認済み（arm64・パッケージング/起動まで確認）／Linux は未検証**（Linux 対応着手時に検証すること）。
- **注意**: `scripts/build.ps1` は PowerShell 5.1 が BOM 無し UTF-8 の日本語コメントを誤読する問題を避けるため、**コメントを ASCII（英語）で記述**している。編集時もこの方針を維持する。

## ツールチェーン（Node/npm 固定）

- **目的**: プラットフォーム間で node/npm のバージョンが異なると `frontend/package-lock.json` が再生成され差分が出る。これを防ぐためフロントエンドのツールチェーンを固定する。
- **固定先**: `frontend/package.json` の `engines`（`node >=24 <25` / `npm >=11 <12`）＋ `volta`（`node 24.16.0` / `npm 11.13.0`）。`frontend/.npmrc` の `engine-strict=true` で範囲外を**エラー**にする。
- **推奨運用**: [Volta](https://volta.sh/) を導入すると `volta` フィールドを読んで node/npm が**自動切替**される（プロジェクト単位・他プロジェクトと無競合）。CI 等 Volta 非導入環境では固定範囲の node/npm を別途用意する。
- **ロックを書き換えない運用**: `wails.json` の `frontend:install` は **`npm ci`**（ロックどおりに厳密インストールし `package-lock.json` を書き換えない）。依存の追加・更新時のみ、固定版の環境で手動 `npm install` → ロック更新をコミットする。`package.json` の `engines` を変更した場合は固定版の npm で一度 `npm install` を回してロックの root `engines` を同期させること。
- **ビルドスクリプトの事前検査**: `build.ps1`/`build.sh` は冒頭で `frontend/package.json` の `volta` を読み、実行環境の node/npm メジャーと照合して不一致なら停止する（`engine-strict` の分かりにくいエラーより前に明示メッセージを出す）。`volta` フィールドが固定バージョンの単一の正。
