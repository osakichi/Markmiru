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
- **開発時の補助コマンド**（最終確認は上記の正式ビルドで行うこと）:
  - `wails dev` … ホットリロード付きの開発実行（フロント変更が即反映）。素早い試行に使う。
  - フロントの型・Svelte チェック: `frontend/` で `npm run check`（`svelte-check`）。
  - Go のコンパイル確認のみ: `go build ./...`（成果物を残さない）。
  - **自動テストは未導入**（Go・フロントとも）。検証は正式ビルドで生成した成果物の手動動作確認で行う。

## アプリ概要

Markdown ドキュメントの**閲覧・編集**を行うデスクトップアプリ「**Markmiru**」（Markdown + 見る）。

- 綺麗にレンダリングして表示する「**閲覧モード**」と、Markdown 表記を直接編集する「**編集モード**」を切り替えて使う
- どちらかといえば**閲覧に重点**を置く
- セッション復元時（前回開いていたファイルを再度開く場合）は**常に閲覧モード**で開く（終了時のモードは保存しない）
- 信頼できない Markdown を開いても安全なよう防御を行う（HTML サニタイズ・CSP・外部リンクは確認ダイアログ後に OS ブラウザへ委譲・外部画像はファイルごとに表示確認）
- 画像はローカルパス（相対・絶対・ルート相対）を data URI 化して表示し、外部（リモート）画像はファイルごとに表示可否を確認する
- 多重起動は防止し、2 つ目の起動は既存ウィンドウにファイルを渡して前面化する（単一インスタンス）

## コード構成・アーキテクチャ

**薄い Go バックエンド ＋ Svelte 5 フロント（OS 標準 WebView 内で動作）**の二層構成。機能の大半はフロントにあり、Go は I/O・ネイティブメニュー・ウィンドウ／単一インスタンス管理のみを担う。設計の正は `docs/アーキテクチャ・画面設計.md`（節番号がコード中のコメントから参照される）。

### Go バックエンド（リポジトリ直下、`package main`）

- `main.go` — エントリポイント。`buildMenu` がネイティブメニューを構築するが、**大半のメニュー項目は処理を持たず `app.emit("menu:*")` でフロントへイベント委譲する**（実処理はフロント側）。例外として、非 macOS の「終了」は Go 側で `runtime.Quit` を直接呼び、macOS は `menu.AppMenu()` / `menu.EditMenu()` のネイティブ標準メニューが直接処理する。WebView アセットは `//go:embed all:frontend/dist`。
- `app.go` — `App` 構造体と Wails にバインドされる**公開メソッド**（`OpenFiles` / `ReadFile` / `SaveFile` / `ReadImageAsDataURL` / クリップボード等）。小文字始まりは非公開（`emit` 等）。`LICENSE.md` / `README.md` を `//go:embed` し、ヘルプメニューの「ライセンス」「Markmiru について（About 代わり）」として返す。
- `config.go` — `config.json`（`os.UserConfigDir()/Markmiru/`）への設定・セッション永続化。標準ライブラリのみ。**ウィンドウ状態は Go 側 `saveWindowState` のみが更新し、フロントの `SaveConfig` は既存値を保持する**（二重管理の衝突回避）。ユーザースタイルは Go では型を持たず JSON 文字列のまま保持する。
- `platform_*.go`（`_windows` / `_darwin` / `_linux` / `_other`）— OS 依存処理（ウィンドウ前面化・WebView フォーカス等）をビルドタグで切替。`isMacOS` 等の定数でメニューを分岐する。
- `single_instance.go` — Unix ソケット（`os.UserCacheDir()/Markmiru/`）による多重起動防止＋IPC。2 つ目の起動は既存へパスを渡して終了し、既存ウィンドウを前面化する（ペイロードは上限検証あり）。

### Svelte フロント（`frontend/src/`）

Svelte 5 の **runes（`$state` / `$derived` / `$effect`）** を使用。状態を持つストアは `.svelte.ts` 拡張子。

- `App.svelte` — ルート。`onMount` でメニューハンドラ登録・リンクハンドラ設置・セッション復元・保留ファイルのオープンを行い、`$effect` で未保存有無とモードを Go に通知し、設定をデバウンス永続化する。
- `lib/commands.ts` — 開く/保存/印刷/終了/スタイル入出力/セッション復元などの**ユーザー操作（コマンド）の中心**。ツールバーとメニューの両方から呼ぶ。終了・タブクローズは未保存時に3択（保存/破棄/キャンセル）。
- `lib/menu.ts` — Go の `menu:*` / `ipc:open-file` / `app:request-quit` イベントを `EventsOn` で購読し、`commands.ts` / `editActions.ts` のコマンドへ接続する。**「Go メニュー → イベント → フロントコマンド」の配線箇所**。
- `lib/api/wails.ts` — 生成バインディング（`wailsjs/`）を型付きでラップした Go 呼び出し層。
- `lib/stores/` — `tabs.ts`（`Tab` モデルと `isDirty`）/ `tabs.svelte.ts`（タブ集合ストア）/ `ui.svelte.ts`（サイドバー・復元中フラグ等）。
- `lib/markdown/renderer.ts` — 閲覧モードのレンダリングパイプライン：**markdown-it（GFM）→ highlight.js / mermaid プレースホルダ → DOMPurify でサニタイズ → DOM 反映後に `mermaid.run()`**。見出しに GitHub 互換スラッグ id を付与。
- `lib/components/` — `TabBar` / `Toolbar` / `Preview`（閲覧）/ `Editor`（CodeMirror 6 編集）/ `SettingsPanel`（スタイル GUI）/ 各種ダイアログ。
- `lib/style/` — テーマ／スタイルの定義・適用（`style.svelte.ts` / `styleDef.ts`）、フォント、ハイライトテーマ。スタイルは JSON でエクスポート/インポート可。設計は `docs/スタイル設定設計.md`。

### 横断する設計上の要点

- **モードは閲覧（`view`）重視。** セッション復元時は終了時のモードに関わらず常に閲覧モードで開く（終了時のモードは保存しない）。
- **セキュリティ防御**：DOMPurify でサニタイズ、本番ビルドのみ CSP 注入、外部リンクはクリック時に確認ダイアログを挟み `BrowserOpenURL` で OS ブラウザへ委譲（`lib/links.ts`）、外部（リモート）画像はファイルごとに表示可否を確認。ローカル画像は Go の `ReadImageAsDataURL` で data URI 化（相対・絶対・ルート相対パス対応）。
- **メニュー処理の OS 分担**：macOS は `menu.EditMenu()` のネイティブ標準編集メニュー（ラベルは英語固定の既知制約）。Windows / Linux は日本語ラベルで手組みし、編集専用項目は閲覧モードで非活性化する（`SetEditMenuEnabled`）。

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
| SHOULD | クロスプラットフォーム対応 | 🔄 コード対応済み（Windows / macOS〔arm64〕動作確認済み・Linux 未検証） |
| SHOULD | Markdown の編集ができる（編集モードでの編集） | ✅ 実装済み |
| SHOULD | 「閲覧モード」と「編集モード」を切り替えられる | ✅ 実装済み |
| SHOULD | 不正な Markdown への防御（CSP・外部リンク制御・外部画像の表示確認・DOMPurify） | ✅ 実装済み |
| SHOULD | ローカル画像の表示（相対・絶対・ルート相対パス、data URI 化） | ✅ 実装済み |
| SHOULD | 多重起動の防止（単一インスタンス、2 つ目の起動は既存へファイル受け渡し） | ✅ 実装済み |

## 対象プラットフォーム

- **初回対象: Windows**
- **最終目標: Windows / macOS / Linux（デスクトップ3種）**
- モバイル（iPhone / Android）は**対象外**（Wails 採用に伴いモバイル不可。この前提で確定）
- 配布形態: ビルド成果物をまとめた**簡素なアーカイブ**（Windows: `.exe` を ZIP／macOS: `.app` を ZIP／Linux: AppImage〔仮決め〕）。インストーラ形式は採らない

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
- 配布物作成: Windows / macOS は `build.ps1` / `build.sh` が `dist/` に ZIP を出力済み（Windows=.exe / macOS=.app を ditto）。**Linux は AppImage を仮決め（未実装）**＝Linux 対応着手時に実装・最終決定する

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
- **ビルドスクリプト**: `scripts/build.ps1`（Windows・動作確認済み）。`scripts/build.sh`（macOS/Linux 用、同等処理）は **macOS で動作確認済み（arm64・ビルド／起動／アプリ機能まで実機検証）／Linux は未検証**（Linux 対応着手時に検証すること）。
- **注意**: `scripts/build.ps1` は PowerShell 5.1 が BOM 無し UTF-8 の日本語コメントを誤読する問題を避けるため、**コメントを ASCII（英語）で記述**している。編集時もこの方針を維持する。
- **配布物（`dist/`）**: ビルド成功後、両スクリプトが `dist/Markmiru-<platform>-<arch>-<sha>-<yyyymmdd>.zip` を出力する（`<sha>`＝版と同じ git ショート SHA、`<yyyymmdd>`＝作成日）。Windows=`.exe` を `Compress-Archive`、macOS=`.app` を `ditto -c -k --keepParent`（バンドルの権限/シンボリックリンク保持）。**Linux は AppImage 仮決めのため未実装でスキップ**。`dist/` は `.gitignore` 済み（配布物はコミットしない）。

## ツールチェーン（Node/npm 固定）

- **目的**: プラットフォーム間で node/npm のバージョンが異なると `frontend/package-lock.json` が再生成され差分が出る。これを防ぐためフロントエンドのツールチェーンを固定する。
- **固定先**: `frontend/package.json` の `engines`（`node >=24 <25` / `npm >=11 <12`）＋ `volta`（`node 24.16.0` / `npm 11.13.0`）。`frontend/.npmrc` の `engine-strict=true` で範囲外を**エラー**にする。
- **推奨運用**: [Volta](https://volta.sh/) を導入すると `volta` フィールドを読んで node/npm が**自動切替**される（プロジェクト単位・他プロジェクトと無競合）。CI 等 Volta 非導入環境では固定範囲の node/npm を別途用意する。
- **ロックを書き換えない運用**: `wails.json` の `frontend:install` は **`npm ci`**（ロックどおりに厳密インストールし `package-lock.json` を書き換えない）。依存の追加・更新時のみ、固定版の環境で手動 `npm install` → ロック更新をコミットする。`package.json` の `engines` を変更した場合は固定版の npm で一度 `npm install` を回してロックの root `engines` を同期させること。
- **ビルドスクリプトの事前検査**: `build.ps1`/`build.sh` は冒頭で `frontend/package.json` の `volta` を読み、実行環境の node/npm メジャーと照合して不一致なら停止する（`engine-strict` の分かりにくいエラーより前に明示メッセージを出す）。`volta` フィールドが固定バージョンの単一の正。
