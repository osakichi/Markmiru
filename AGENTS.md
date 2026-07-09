# AGENTS.md

このファイルは、本プロジェクト（Markmiru）で作業する際のガイダンスです。

## プロジェクト規約

- **やりとりはすべて日本語で行う。**
- **生成する `.md` ファイルもすべて日本語で保存する。**（応答文・コメント・ドキュメント・Markdown ファイルなど、出力するテキストは日本語）
- 不明な点は推測で進めず、必ず確認する。仕様・方針・選択肢の選定はユーザーに質問する。
- **コミットメッセージは英語、1〜2行。箇条書きでの更新内容の説明は不要。**`Co-Authored-By:` や `Generated with` などの自動付記は不要。
- **リポジトリに含めるのは「これがなければ最終生成物を作れない」ソースのみ。** Go モジュールキャッシュなどの外部取得物、ビルド成果物（`build/bin/` 配下の `.exe` / `.app` 等）、配布物（`dist/` の ZIP）は含めない。ただし自動生成されたプレースホルダー（アイコン等、後で独自に差し替える前提のもの）や、`web/assets/fonts/` に vendoring 済みの同梱フォント woff2（`scripts/vendor-fonts.sh` の生成物・通常ビルドに再生成不要）は含める。
- **最終生成物（`build/bin/` 配下の実行体）に影響する変更を行った場合は、必ず正式ビルドスクリプトでビルドが通ることを確認する。** フロントエンドのみの変更であっても例外なく実施する。**動作確認・検証も必ずこの正式ビルドで生成した成果物に対して行う**（素のバイナリで確認しない）。この規約は **Windows / macOS / Linux すべてに適用**する。
  - ビルドコマンド: **Windows** = `& .\scripts\build.ps1` / **macOS・Linux** = `./scripts/build.sh`（または `bash ./scripts/build.sh`）。いずれも git ショート SHA をバージョンとして埋め込むラッパーで、内部で `wails build` を実行し、**成果物は必ず `build/bin/` に出力される**（プロジェクト直下には作らない）。
  - 素の `wails build` でも生成自体は可能だが、その場合バージョンは `dev` になる。
  - **引数なしの `go build` は使わない**（Go の仕様でプロジェクト直下に実行体を生成し作業ツリーを汚すため）。コンパイルが通るかの確認だけなら成果物を残さない `go build ./...` を用いる。実行可能なバイナリが必要な場合も出力先は必ず `build/bin/` 配下にする（`go build -o build/bin/Markmiru .`）。
- **開発時の補助コマンド**（最終確認は上記の正式ビルドで行うこと）:
  - `wails dev` … 開発実行に使う。ただし**本プロジェクトでは HMR（Hot Module Replacement）を開発中も使用しない**方針。素早い反映が要るときは Go 再ビルド＋WebView リロードで行う（フロントエンドのバンドル工程が無く、そもそもフロント HMR は存在しない）。
  - Go のコンパイル確認のみ: `go build ./...`（成果物を残さない）。
  - `web` パッケージのテスト: `go test ./web/`（HTTP ハンドラの単体テスト）。全体は `go test ./...`。整形は `gofmt -w`、静的検査は `go vet ./...`。
  - 検証は正式ビルドで生成した成果物の手動動作確認でも行う。

## アプリ概要

Markdown ドキュメントの**閲覧・編集**を行うデスクトップアプリ「**Markmiru**」（Markdown + 見る）。

- 綺麗にレンダリングして表示する「**閲覧モード**」と、Markdown 表記を直接編集する「**編集モード**」を切り替えて使う
- どちらかといえば**閲覧に重点**を置く
- セッション復元時（前回開いていたファイルを再度開く場合）は**常に閲覧モード**で開く（終了時のモードは保存しない）
- 信頼できない Markdown を開いても安全なよう防御を行う（HTML サニタイズ・CSP・外部リンクは確認ダイアログ後に OS ブラウザへ委譲・外部画像はファイルごとに表示確認）
- 画像はローカルパス（相対・絶対・ルート相対）を data URI 化して表示し、外部（リモート）画像はファイルごとに表示可否を確認する。文書と別ホストを指す UNC 画像（`\\別ホスト\…`）は認証情報漏洩（別サーバへの SMB 自動認証）を防ぐため確認を出さず常に遮断する（同一ホスト UNC は許容）
- 多重起動は防止し、2 つ目の起動は既存ウィンドウにファイルを渡して前面化する（単一インスタンス）

## コード構成・アーキテクチャ

**厚い Go バックエンド ＋ 薄い WebView 表示層（Go-SSR）**の構成。状態・描画・HTML 生成・アプリロジックはすべて Go にあり、WebView 内の自作物は **glue.js（1ファイル）** のみ（既製ライブラリは htmx・mermaid.js）。Wails の `AssetServer.Handler`（`http.Handler`）に Go のルータを載せ、htmx からの相対 URL 要求に **HTML 断片**を返す（TCP なしのプロセス内疑似 HTTP）。設計・画面仕様・エンドポイント一覧は `docs/アーキテクチャ・画面設計.md`。技術選定の経緯は `docs/技術選定.md`／`docs/代替技術調査.md`。

### Go バックエンド

- `main.go` — エントリポイント。`buildMenu` がネイティブメニューを構築し、大半の項目は `app.emit("menu:*")` でイベントを発火する（glue が htmx.ajax で Go-SSR エンドポイントに橋渡し）。起動時に `web.State` をつくり config からセッション/スタイル/サイドバーを復元、`webHost{app}` を注入して `web.NewHandler` を `AssetServer.Handler` に載せる。非 macOS の「終了」は `runtime.Quit`、macOS は `menu.AppMenu()` / `menu.EditMenu()` のネイティブ標準メニュー。
- `web`（新規パッケージ）— **Go-SSR の中心**。`http.Handler`（`web.go`：全 htmx エンドポイント＋CSP ミドルウェア）、`html/template` フラグメント（`web/templates/*.html`）、アプリ状態（`web/state.go`：タブ集合／アクティブ／dirty／セッション／スタイル）、静的アセット（`web/assets/`：`glue.js` / `htmx.min.js` / `mermaid.min.js` / CSS / 同梱フォント）。OS 連携は `web.Host` インタフェース経由で `main` の `webHost` アダプタが `app.go` のメソッドへ委譲する。
- `render`（新規）— 描画パイプライン **goldmark（GFM・脚注）→ chroma（コードハイライト）→ bluemonday（サニタイズ）**。mermaid はプレースホルダのまま返し WebView の mermaid.js が描画。ローカル画像の data URI 化・外部画像の遮断制御もここ。
- `style`（新規）— スタイル定義（Go 型）→ CSS 変数生成。プリセット・エクスポート/インポート。設計は `docs/スタイル設定設計.md`。
- `app.go` — `App` 構造体と Wails バインドの**公開メソッド**（`OpenFiles` / `ReadFile` / `SaveFile` / `SaveFileDialog` / `ExportStyleDialog` / `ImportStyleDialog` / `SetEditMenuEnabled` / `SetSaveMenuEnabled` / `SetModeMenuEnabled` / `OpenExternalURL` / `Quit` / `FocusWindow` / `Print` 等、OS 機能に限定）。小文字始まりは非公開（`emit` / `openFileFromIPC` 等）。`LICENSE.md` / `README.md` を `//go:embed`。
- `config.go` — `config.json`（`os.UserConfigDir()/Markmiru/`）への設定・セッション永続化。標準ライブラリのみ。ウィンドウ状態は `saveWindowState`、セッション/スタイル/サイドバーは `beforeClose` の `persistSession`（`web.State` から生成・main で注入）が保存する。
- `platform_*.go`（`_windows` / `_darwin` / `_linux` / `_other`）— OS 依存処理（ウィンドウ前面化・WebView フォーカス・印刷 `platformPrint` 等）をビルドタグで切替。darwin のみ cgo（Objective-C）で自前のネイティブ印刷（縦向きデフォルト）を実装し、他 OS は未処理を返して Wails `WindowPrint` にフォールバックする。`isMacOS` 等でメニューを分岐。
- `single_instance.go` — Unix ソケット（`os.UserCacheDir()/Markmiru/`）による多重起動防止＋IPC。2 つ目の起動は既存へパスを渡して終了、既存ウィンドウを前面化。既存アプリでは `ipc:open-file` イベント→glue→`POST /tabs/open-path` でそのファイルを開く。

### WebView 表示層（`web/assets/`）

- `glue.js` — WebView 内の唯一の自作 JS。「仕組み的グルー」に限定: mermaid 再描画、編集オーバーレイのスクロール同期、保存前の未送信編集 flush（menu:save/saveAs・Ctrl+S フォールバック）、ネイティブメニュー/IPC の Wails イベント（`window.runtime.EventsOn('menu:*' / 'ipc:open-file' / 'app:request-quit')`）→ htmx.ajax ブリッジ、ダイアログのフォーカス/Esc/危険確認の遅延活性化、ページ内検索（CSS Custom Highlight API）、外部リンクの遷移制御、印刷トリガ、設定の対入力同期。
- htmx / mermaid.min.js — 既製ライブラリ（vendoring 済み）。htmx が操作→Go 要求→DOM 断片差し替えを担う。
- CSS（`app.css` / `markdown.css`）＋ 同梱フォント（`fonts.css` / `fonts/*.woff2`）。本文配色・組版は Go 出力のスタイル変数（`#styleblock`）。

### 横断する設計上の要点

- **モードは閲覧（`view`）重視。** セッション復元時は終了時のモードに関わらず常に閲覧モードで開く（終了時のモードは保存しない）。編集モードは**透明 `<textarea>` に chroma ハイライト層を重ねるオーバーレイ**（CodeMirror ではない）。
- **セキュリティ防御**：bluemonday でサニタイズ、**CSP をミドルウェア（`web.NewHandler`）で全レスポンスに注入**（`default-src 'none'; script-src 'self' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' data: http: https:; font-src 'self' data:; connect-src 'self'; …`。`unsafe-eval` は mermaid/htmx の Function 用）。外部リンクはクリック時に確認ダイアログを挟み `OpenExternalURL`（`BrowserOpenURL`）で OS ブラウザへ委譲。外部画像はファイルごとに表示可否を確認。ローカル画像は `render` パッケージが data URI 化（相対・絶対・ルート相対パス対応）。
- **メニュー処理の OS 分担**：macOS は `menu.EditMenu()` のネイティブ標準編集メニュー（ラベルは英語固定の既知制約）。Windows / Linux は日本語ラベルで手組みし、編集専用項目は閲覧モードで非活性化する。「ファイル → 保存」は dirty または無題のときだけ有効（全 OS）。**有効/無効はサーバ主導**：`web` のミドルウェア（`withMenuSync`）が各 POST 後に状態から一括判定し、変化時だけ `Host.SetEditMenuEnabled` / `SetSaveMenuEnabled` を呼ぶ（フロント通知ではない）。

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
| SHOULD | クロスプラットフォーム対応 | ✅ 実装済み（Windows / macOS〔arm64〕/ Linux〔amd64・Ubuntu 24.04〕で動作確認済み） |
| SHOULD | Markdown の編集ができる（編集モードでの編集） | ✅ 実装済み |
| SHOULD | 「閲覧モード」と「編集モード」を切り替えられる | ✅ 実装済み |
| SHOULD | 不正な Markdown への防御（CSP・外部リンク制御・外部画像の表示確認・bluemonday サニタイズ） | ✅ 実装済み |
| SHOULD | ローカル画像の表示（相対・絶対・ルート相対パス、data URI 化） | ✅ 実装済み |
| SHOULD | 多重起動の防止（単一インスタンス、2 つ目の起動は既存へファイル受け渡し） | ✅ 実装済み |

## 対象プラットフォーム

- **初回対象: Windows**
- **最終目標: Windows / macOS / Linux（デスクトップ3種）**
- モバイル（iPhone / Android）は**対象外**（Wails 採用に伴いモバイル不可。この前提で確定）
- 配布形態: ビルド成果物をまとめた**簡素なアーカイブ**（Windows: `.exe` を ZIP／macOS: `.app` を ZIP／Linux: バイナリを tar.gz）。インストーラ形式は採らない

## 技術スタック

| レイヤー | 採用技術 | 備考 |
|----------|----------|------|
| 実装基盤 | **Wails v2**（安定版） | Go バックエンド ＋ OS 標準 WebView。v3 はアルファのため不採用 |
| バックエンド言語 | **Go**（厚い層） | 状態管理・Markdown 描画・HTML 生成・ファイル I/O・タブ/ウィンドウ管理・ネイティブメニュー・PDF 出力起動 |
| WebView | OS 標準 | Win=WebView2 / macOS=WKWebView / Linux=WebKitGTK |
| UI 方式 | **Go-SSR（`html/template`）＋ htmx** | Go が HTML 断片を生成、htmx が DOM 差し替え。自作 JS は glue.js のみ |
| Markdown 解析 | **goldmark**（Go・GFM＋脚注） | CommonMark＋GFM・脚注。拡張はレンダラで実装 |
| 図表 | **mermaid.js** | WebView 内でそのまま描画。`securityLevel: 'strict'` |
| コードハイライト | **chroma**（Go） | `WithClasses`。github / github-dark テーマ |
| 編集モード | **透明 textarea オーバーレイ ＋ chroma ハイライト層** | 軽量。行番号等は持たない（閲覧重視） |
| サニタイズ | **bluemonday**（Go） | 許可リストで無害化（script 等除去）。外部リンクへの `rel` 付与・外部画像の遮断は `render` パッケージが実施 |
| セキュリティ | **CSP**（ミドルウェアで全レスポンスに注入） | `default-src 'none'; script-src 'self' 'unsafe-eval'; …`。外部リンクは確認ダイアログ後 `BrowserOpenURL` |
| PDF 出力 | OS / WebView の印刷 → PDF | 専用ライブラリ不要 |
| スタイル変更 | Go 生成の CSS 変数（テーマ切替） | 設定はモーダルで GUI 編集。JSON 入出力はメニュー「ファイル → スタイル」 |
| 同梱フォント | **Noto Sans/Serif/Mono JP**（woff2 を直 vendor） | `web/assets/fonts/` に vendoring 済み |
| マルチタブ | Go 状態（`web/state.go`） | 1ウィンドウ内のタブバーで管理 |

## 選定理由（要約）

- **Wails**: mermaid を WebView でそのまま描画でき（MUST）、Electron と違い OS 標準 WebView を使うため起動が速く軽量（MUST）。Win/mac/Linux 対応で Go で書ける。安定版の v2 を採用。
- **不採用**: Electron（起動が重い）、Tauri v2（同等候補だが Go の好みを優先）、Fyne/Gio（mermaid 描画困難）、Flutter/.NET MAUI（mermaid・Linux 対応で不利）。

詳細は `docs/技術選定.md` を参照。

## 残タスク

- 各設計ドキュメント（`docs/`）の Go-SSR 構成への追随（順次更新）。

## 埋め込みドキュメント（ヘルプメニュー）

- **ライセンス**: Markmiru 自体＋サードパーティ（Wails・goldmark・chroma・bluemonday・htmx・mermaid・Noto フォント〔OFL〕等）のライセンスを `LICENSE.md`（リポジトリ直下）に統合。`//go:embed` で実行バイナリに埋め込む（別ファイル配置は不要）。ネイティブメニュー「ヘルプ → ライセンス...」→ `menu:license` イベント → glue → `POST /doc/license` で編集不可タブとして表示（内容は `ReadLicense()`）。
- **About（README）**: `README.md`（リポジトリ直下）も `//go:embed`。「ヘルプ → Markmiru について...」→ `menu:about` → `POST /doc/about` で編集不可タブ表示（`ReadReadme()`。専用 About 画面は設けず README を代替とする）。
- いずれも `filePath=null` の readOnly タブで開く。セッションには残さない（表示中はタブ・サイドバーの「開いているファイル一覧」には他のタブと同様に現れる）。

## アプリアイコン

- **Windows**: `build/windows/icon.ico` を差し替えればそのまま埋め込まれる（Wails は既存なら再生成しない。無い場合のみ `build/appicon.png` から生成）。
- **macOS**: Wails は `.icns` を**常に `build/appicon.png` から生成**し、既製 `.icns` を読み込む口がない。そのため手作りの `build/darwin/iconfile.icns` を**ビルド後フックで .app バンドルへ上書きコピー**する方式を採用（`wails.json` の `postBuildHooks` → `darwin/*`）。フックは作業ディレクトリ `build/bin`・シェル非経由で実行されるため、コマンドは `cp ../darwin/iconfile.icns Markmiru.app/Contents/Resources/iconfile.icns`。非ネイティブ（Windows 上での darwin 指定等）では自動スキップされる。

## バージョン番号

- **版＝git のショート SHA**（作業ツリーが汚れている＝`git status --porcelain` が非空なら `-dirty` を付与）。
- **埋め込み方法（ビルドスクリプトが両方を実施）**:
  - **OS プロパティ**: `wails.json` の `info.productVersion` に SHA を一時注入 → Wails が `build/windows/info.json`（Windows）・`build/darwin/Info.plist`（macOS）へ展開して実行バイナリに埋め込む。注入後は `wails.json` を**元に戻す**（リポジトリに SHA 差分を残さない。`info.productVersion` の既定は `dev`）。Windows「詳細 → 製品バージョン」に表示（`File version` は数値固定 `0.0.0.0`＝`build/windows/info.json` の `fixed.file_version`）。
  - **実行時表示**: Go の ldflags `-X main.version=<sha>`（`main.version` の既定は `dev`）。`ReadReadme()` が About（README）タブ先頭に「バージョン: <sha>」を追記。
- **ビルドスクリプト**: `scripts/build.ps1`（Windows・動作確認済み）。`scripts/build.sh`（macOS/Linux 用、同等処理）は **macOS（arm64）・Linux（amd64・Ubuntu 24.04）で動作確認済み**（いずれも Go-SSR 版でビルド／起動／アプリ機能まで実機検証）。Linux では WebKitGTK 4.1 の有無を pkg-config で自動判別し、あれば `-tags webkit2_41` を付与する。
- **注意**: `scripts/build.ps1` は PowerShell 5.1 が BOM 無し UTF-8 の日本語コメントを誤読する問題を避けるため、**コメントを ASCII（英語）で記述**している。編集時もこの方針を維持する。
- **配布物（`dist/`）**: ビルド成功後、両スクリプトが `dist/Markmiru-<platform>-<arch>-<sha>-<yyyymmdd>.zip` を出力する（`<sha>`＝版と同じ git ショート SHA、`<yyyymmdd>`＝作成日）。Windows=`.exe` を `Compress-Archive`、macOS=`.app` を `ditto -c -k --keepParent`（バンドルの権限/シンボリックリンク保持）、Linux=バイナリのみを `tar.gz`（実行権限保持のため ZIP ではなく tar.gz。拡張子は `.tar.gz`）。`dist/` は `.gitignore` 済み（配布物はコミットしない）。`<arch>` は `build.sh` が `uname -m` から取得する一方、**`build.ps1` は `windows-amd64` 固定**（アーキテクチャを検出しない。ARM Windows 対応時は要修正）。

## ビルドツールチェーン（Go のみ）

- 本アプリは Go-SSR（Wails `AssetServer.Handler`）で、**ビルドすべきフロントエンドは無い**（`wails.json` にフロントフックは無く、`wails build` は "No Install/Build command. Skipping." で Go のみをコンパイルする。`frontend/wailsjs` は `wails build` が再生成するバインディングで `.gitignore` により git 管理外）。
- 必要なのは **Go（`go.mod` の `go 1.25`）・Wails CLI v2・git** のみ。`scripts/build.ps1` / `scripts/build.sh` は git SHA を埋め込み `wails build` を実行して `dist/` に ZIP を出力する。**両スクリプトは版取得に `git rev-parse` / `git status` を使うため、git リポジトリの作業ツリー内で実行する必要がある**（非 git 環境では SHA 取得に失敗して停止。版を埋め込まない素の `wails build`〔版は `dev`〕は非 git でも可）。
- **macOS は SDK 11 以上が必須**: Wails v2 の `WailsContext.m` が macOS 11 で追加された通知定数（`UNNotificationPresentationOptionList` / `Banner`）を参照するため、Command Line Tools の SDK が 10.15 以前だと undeclared identifier でコンパイル失敗する（`@available` は実行時チェックのみでコンパイルは通らない）。`xcrun --show-sdk-version` で確認し、古ければ CLT を入れ直す（2026-07 に古い Intel Mac で発生・CLT 再インストールで解消済み）。
- **ビルドスクリプトは `wails` を固定パスで参照する**（`build.ps1`＝`%USERPROFILE%\go\bin\wails.exe` / `build.sh`＝`$HOME/go/bin/wails`。`PATH` は参照しない）。`GOPATH` / `GOBIN` を変更した環境ではそのままでは動かないため、スクリプト内のパスを環境に合わせる必要がある。
- 同梱フォントの woff2 は `web/assets/fonts/` に vendoring 済み（リポジトリにコミット）。更新時のみ `scripts/vendor-fonts.sh`（@fontsource が必要）で再生成する。
