# Markmiru

Markdown ドキュメントの**閲覧・編集**を行うデスクトップアプリ（**Markmiru** = Markdown ＋ 見る）。

## 概要

**Markmiru** は、Markdown を**素早く・簡単に**閲覧することに重点を置いたデスクトップアプリです。Markdown を綺麗にレンダリングして見る「**閲覧モード**」と、簡単な修正等のため Markdown 記法を直接編集する「**編集モード**」を備えています。

- mermaid 図・コードハイライト・GFM（表・チェックリスト等）に対応
- Wails v2（Go ＋ OS 標準 WebView）採用で、Electron に比べ**軽量・高速起動**
- プラグイン不要の**単体アプリ**として動作
- 信頼できない Markdown を開いても安全なよう**防御**（HTML サニタイズ・CSP・外部リンクは確認ダイアログ後に OS ブラウザへ委譲・外部画像はファイルごとに表示確認）

## 機能一覧

- **マルチタブ**で複数ドキュメントを切り替えて表示
- **閲覧モード**：markdown-it によるレンダリング表示
  - mermaid 図のレンダリング（`securityLevel: 'strict'`）
  - highlight.js によるコードシンタックスハイライト
  - GFM（表・チェックリスト・打消し線・自動リンク）
- **編集モード**：CodeMirror 6（行番号・ソフトラップ・控えめな構文ハイライト）
- 閲覧／編集モードの切り替え（セッション復元時は**常に閲覧モード**で開く）
- **ページ内検索**（Ctrl+F／右クリックメニュー。閲覧・編集の両モード）
- **右クリックメニュー**（コピー／貼り付け／リンクのコピー 等。両モード）
- **スタイル**による配色（ライト／ダーク）／表示のカスタマイズ（ライト／ダーク／GitHub 風／セピアのプリセットを複製して編集可）
  - 本文・見出し・コード・引用・リスト・表などの各パラメータを GUI 設定パネルで調整
  - カスタム CSS による上書き（上級者向け）
  - スタイルのエクスポート / インポート（JSON ファイル。別マシン・別 OS へ持ち運び可能）
- **PDF 出力 / 印刷**（WebView の印刷機能。既定は印刷向け配色に変換、「表示通りに印刷」で画面通り）
- **画像表示**：ローカルパス（相対・絶対・ルート相対）を data URI 化して表示。外部（リモート）画像はファイルごとに表示可否を確認
- **セキュリティ**：DOMPurify によるサニタイズ、本番ビルドでの CSP 注入、外部リンクは確認ダイアログ後に OS ブラウザへ委譲
- **単一インスタンス**：多重起動を防止し、2 つ目の起動は既存ウィンドウへファイルを渡して前面化
- **セッション復元**：前回開いていたファイル群を起動時に再オープン（全件を起動時に読込。不在ファイルは 1 件ずつダイアログ）
- 同梱フォント（Noto Sans JP / Noto Serif JP / Noto Sans Mono）による OS 間の表示一致（等幅の日本語は Noto Sans JP へフォールバック）

## 依存パッケージ / ライブラリ

### バックエンド（Go）

| ライブラリ | 用途 |
|------------|------|
| [Wails v2](https://wails.io/)（`github.com/wailsapp/wails/v2`） | アプリ基盤（Go ＋ OS 標準 WebView） |
| `golang.org/x/sys` | OS 依存処理（macOS/Linux の単一インスタンス IPC のピア検証） |
| Go 標準ライブラリ（`os` / `encoding/json` / `path/filepath` 等） | ファイル I/O・設定永続化 |

### フロントエンド（Svelte / WebView）

| ライブラリ | 用途 |
|------------|------|
| [Svelte](https://svelte.dev/) ＋ [Vite](https://vitejs.dev/) ＋ TypeScript | UI フレームワーク・ビルド |
| [markdown-it](https://github.com/markdown-it/markdown-it) ＋ markdown-it-task-lists | Markdown 解析 |
| [mermaid](https://mermaid.js.org/) | 図表のレンダリング |
| [highlight.js](https://highlightjs.org/) | コードシンタックスハイライト |
| [CodeMirror 6](https://codemirror.net/)（`@codemirror/view` / `state` / `commands` / `language` / `autocomplete` / `search` / `lang-markdown` / `theme-one-dark`） | 編集モードのエディタ（必要な拡張のみ自前構成） |
| [DOMPurify](https://github.com/cure53/DOMPurify) | レンダリング HTML のサニタイズ |
| [@fontsource](https://fontsource.org/) Noto Sans JP / Noto Serif JP / Noto Sans Mono | 同梱フォント（自己ホスト） |

> 正確なバージョンは [`go.mod`](go.mod) と [`frontend/package.json`](frontend/package.json) を参照してください。ライセンス表記は [`LICENSE.md`](LICENSE.md) に統合しています。

## 対応プラットフォーム

| プラットフォーム | 状態 |
|------------------|------|
| **Windows** | ✅ 対応・動作確認済み（WebView2） |
| **macOS** | ✅ 対応・**動作確認済み**（arm64。ビルド・起動・アプリ機能まで実機検証）（WKWebView） |
| **Linux** | 🔄 コード対応済み・**未検証**（WebKitGTK） |

- 最終目標は Windows / macOS / Linux のデスクトップ 3 種対応です。
- モバイル（iPhone / Android）は対象外です（Wails 採用のため）。
- **macOS は arm64 でビルド・起動・アプリ機能まで実機で確認済みです。Linux の手順は Wails の標準的な要件に基づく想定で、実機での確認は行っていません**（今後整備予定）。以降の各節で Linux はこの前提です（🔄 印で示します）。

---

## インストール方法

> ⚠️ 配布は**インストーラ形式ではなく、ビルド成果物をまとめた簡素なアーカイブ**を予定しています（**Windows**: `.exe` のみを ZIP／**macOS**: `.app` ごと ZIP／**Linux**: 将来対応予定）。現時点では配布物は未提供のため、下記「[ソースからのビルド手順](#ソースからのビルド手順)」でビルドしてご利用ください（ビルドスクリプトは `dist/` に配布用アーカイブを出力します）。

### Windows

ビルド済みの `build\bin\Markmiru.exe` は単体で動作します。任意のフォルダに配置してそのまま実行できます。

- 動作には **WebView2 ランタイム**が必要です。Windows 10 / 11 には標準で同梱されていることがほとんどですが、未導入の場合は [Microsoft Edge WebView2 ランタイム](https://developer.microsoft.com/microsoft-edge/webview2/) を導入してください。

### macOS

- ビルドした **`build/bin/Markmiru.app`** を `/Applications`（アプリケーション）フォルダへドラッグして配置します。
- 署名・公証を行っていないため、初回起動時に Gatekeeper の警告が出る想定です。その場合は Finder でアプリを右クリック →「開く」で実行を許可するか、必要に応じて隔離属性を解除します:
  ```bash
  xattr -dr com.apple.quarantine /Applications/Markmiru.app
  ```

### Linux 🔄

- ビルドした実行ファイル **`build/bin/Markmiru`** を任意のディレクトリ（例: `~/.local/bin`）に配置し、実行権限を付与して起動します:
  ```bash
  chmod +x build/bin/Markmiru
  ./build/bin/Markmiru
  ```
- 実行には **WebKitGTK ランタイム**が必要です（ビルド環境の構築で導入する開発パッケージに含まれます）。配布先の環境では対応するランタイムライブラリの導入が必要になる想定です。
- **配布物（アーカイブ）は将来対応予定**です（現状はビルドした上記バイナリをご利用ください）。

---

## ビルド環境の構築手順

Go・Node.js・Wails CLI の導入は全 OS で共通です。これに加えて、OS ごとにネイティブ依存（WebView ランタイム・C コンパイラ等）の導入が必要です。

### 共通手順

1. **Go（1.23 以上）** をインストール
   - 入手元: [https://go.dev/dl/](https://go.dev/dl/)（各 OS 用インストーラ／アーカイブ）。Homebrew・パッケージマネージャでも可。
   - 確認: `go version`
   - `go install` した実行ファイルは既定で `go\bin`（Windows: `%USERPROFILE%\go\bin` / macOS・Linux: `~/go/bin`）に置かれます。
2. **Node.js ＋ npm**（バージョン固定）をインストール
   - 本プロジェクトは **Node 24 系／npm 11 系**に固定しています（`frontend/package.json` の `volta`・`engines`、および `frontend/.npmrc` の `engine-strict`）。範囲外のバージョンでは依存インストールが**失敗**します（バージョン差による `package-lock.json` の意図しない再生成を防ぐため）。
   - **推奨: [Volta](https://volta.sh/) を導入** → リポジトリ内に入ると `node`/`npm` が固定版へ**自動で切り替わり**ます（他プロジェクトと競合しません。固定版は初回に自動取得）。Volta なら手動でのバージョン管理は不要です。
   - 手動で導入する場合は [https://nodejs.org/](https://nodejs.org/) 等で **Node 24 系（npm 11 同梱）** を入れてください。
   - 確認: `node -v`（`v24.x`）／ `npm -v`（`11.x`）
   - フロントエンドの依存取得はビルド時に Wails が自動実行（`npm ci`）するため、手動での操作は通常不要です（依存を追加・更新したいときのみ、固定版の環境で手動 `npm install` を実行してロックを更新します）。

   <details>
   <summary><b>Volta を使わない場合</b>（既に別バージョンの node/npm が入っている環境）</summary>

   OS が PATH 上に見せる `node`/`npm` は基本的に1組だけです。固定版（Node 24 系／npm 11 系）にするには次のいずれかを取ります（版が範囲外なら `engine-strict` とビルドスクリプトの検査が**エラーで知らせます**）。

   - **A. グローバルの node を入れ替える**（他プロジェクトで別版が不要なら最も簡単）
     - [nodejs.org](https://nodejs.org/) から **Node 24 系**インストーラを入れて既存版を上書き（npm 11 同梱）。必要なら `npm install -g npm@11.13.0` で npm を合わせる。
     - ⚠️ マシン全体の node が変わるため、別版を要する他プロジェクトに影響します。
   - **B. ビルド用シェルだけ固定版を PATH の先頭に出す**（グローバルを変えたくない場合）
     - Node 24 系の**バイナリ（zip／tar。インストーラではない）**を任意フォルダに展開し、ビルドするターミナルでのみ PATH 先頭に追加する:
       - Windows（PowerShell・そのセッションのみ）: `$env:Path = "C:\node24;$env:Path"`
       - macOS / Linux（そのシェルのみ）: `export PATH="$HOME/node24/bin:$PATH"`
     - `node -v` が `v24.x` になっていることを確認してからビルド。他のターミナルは従来の node のままです。
   - **C. 別のバージョンマネージャを使う**
     - 複数プロジェクトで版を共存させたいなら **nvm（nvm-windows）／ fnm／ mise／ asdf** でも可（プロジェクトごとに自動切替）。要件は「固定版が PATH に出ること」だけで、ツールは問いません。
   </details>
3. **Wails CLI** をインストール（コマンドは全 OS 同一）
   ```
   go install github.com/wailsapp/wails/v2/cmd/wails@latest
   ```
   - インストール先は上記 `go\bin`。`go\bin` を `PATH` に通すと `wails` だけで呼び出せます（PATH の通し方は後述の「OS 固有事項」を参照）。
4. **OS 固有のネイティブ依存**（後述）を導入してから、**環境チェック**を実行（コマンドは全 OS 同一）
   ```
   wails doctor
   ```
   - Go / Node.js / npm / WebView ランタイム / C コンパイラ等の状態が一覧表示されます。不足や警告が出た項目は指示に従って導入・修正してください。すべて「OK」になればビルド可能です。

### OS 固有事項

#### Windows

- **WebView2 ランタイム**: 未導入の場合は [Microsoft Edge WebView2 ランタイム](https://developer.microsoft.com/microsoft-edge/webview2/)（「Evergreen Standalone Installer」など）を導入。Windows 10 / 11 には標準で同梱されていることがほとんどです。
- **C コンパイラ（cgo 用）**: [TDM-GCC](https://jmeubank.github.io/tdm-gcc/) もしくは [WinLibs（MinGW-w64）](https://winlibs.com/) を導入し、`gcc` を `PATH` に通す（確認: `gcc --version`）。
- **PATH の通し方**（PowerShell。現在のセッションのみの例）:
  ```powershell
  $env:Path += ";$env:USERPROFILE\go\bin"
  ```
  恒久的に通す場合はシステムの環境変数 `Path` に `%USERPROFILE%\go\bin` を追加します。
- 各インストーラによる `PATH` 変更を反映させるため、確認コマンドは**新しいターミナルを開き直してから**実行してください。

#### macOS

- **Xcode Command Line Tools**（C コンパイラ等）を導入:
  ```bash
  xcode-select --install
  ```
- **WKWebView** は macOS 標準のため追加導入は不要の想定です。
- **PATH の通し方**: `export PATH="$HOME/go/bin:$PATH"` を `~/.zshrc` 等に追記。

#### Linux 🔄

ディストリビューションにより必要パッケージ名は異なります。**C コンパイラ・GTK・WebKitGTK・pkg-config** を導入します。

- Debian / Ubuntu 系:
  ```bash
  sudo apt update
  sudo apt install build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.0-dev
  ```
  - `libwebkit2gtk-4.0-dev` が見つからない場合は `libwebkit2gtk-4.1-dev` を使用（ビルド時に `-tags webkit2_41` を付与）。
- Fedora 系:
  ```bash
  sudo dnf install gcc-c++ pkgconf-pkg-config gtk3-devel webkit2gtk4.1-devel
  ```
- Arch 系:
  ```bash
  sudo pacman -S base-devel gtk3 webkit2gtk pkgconf
  ```
- **PATH の通し方**: `export PATH="$HOME/go/bin:$PATH"` を `~/.bashrc` 等に追記。

---

## ソースからのビルド手順

`git clone` とビルドの中核は全 OS 共通です。成果物のパスと一部のオプションのみ OS で異なります。

### 共通手順

1. リポジトリを取得
   ```
   git clone <このリポジトリの URL> Markmiru
   cd Markmiru
   ```
2. 本番ビルド（**ビルドスクリプト経由を推奨**）
   - Windows（PowerShell）:
     ```powershell
     & .\scripts\build.ps1
     ```
   - macOS / Linux（macOS は arm64 で動作確認済み / Linux は未検証 🔄）:
     ```bash
     ./scripts/build.sh
     ```
   - このスクリプトは **git のショート SHA をバージョンとして埋め込んだ上で内部的に `wails build` を実行**します（埋め込んだ版は「ヘルプ → Markmiru について」やファイルのプロパティ＝製品バージョンで確認できます）。
   - **バージョン埋め込みが不要なら、素の `wails build` でもビルドできます**（その場合バージョンは `dev` 表示になります）。`wails` を `PATH` に通していない場合はフルパスで呼び出します（例: Windows `& "$env:USERPROFILE\go\bin\wails.exe" build` / macOS・Linux `~/go/bin/wails build`）。
   - フロントエンドの依存取得（`npm ci`＝ロックどおりに厳密インストールし `package-lock.json` を書き換えない）・ビルド（`npm run build`）・Go バインディング生成は **Wails が自動で実行**します。
   - ビルドスクリプトは実行時に node/npm が固定版（**Node 24 系・npm 11 系**）かを検査し、不一致なら明示メッセージで停止します（Volta 導入済みなら自動で固定版が使われます）。
   - 成果物は `build/bin/` に出力されます（ファイル名は OS により異なる。後述）。
   - 続けて**配布用アーカイブを `dist/` に出力**します（命名: `Markmiru-<platform>-<arch>-<sha>-<yyyymmdd>.zip`。`<sha>` は上記バージョンと同じ git ショート SHA、`<yyyymmdd>` は作成日）。Windows は `.exe` のみ・macOS は `.app` ごとを ZIP 化。**Linux 用アーカイブは将来対応予定**で現状は作成しません（バイナリは `build/bin/` に残ります）。配布方針は「[インストール方法](#インストール方法)」参照。
   - 初回ビルドは Go モジュール・npm パッケージの取得が走るため時間がかかります（ネットワーク接続が必要）。2 回目以降はキャッシュにより短縮されます。

> **`-clean` / `-debug` / `-platform` / `-tags` 等の `wails build` オプション**を使う場合は、ビルドスクリプトを介さず `wails build <オプション>` を直接実行してください（その場合バージョンは `dev`）。バージョンも埋め込みたいときは `-ldflags "-X main.version=<任意>"` を併用します。

### OS 固有事項

#### Windows

- 成果物: **`build\bin\Markmiru.exe`**（配布は `dist/` の ZIP）
- 固有オプション:
  - `-debug` … デバッグ情報付き（開発者ツールを開けるビルド）

#### macOS

- 成果物: **`build/bin/Markmiru.app`**
- 固有オプション:
  - `-platform darwin/universal` … Apple Silicon ＋ Intel のユニバーサルバイナリを生成

#### Linux 🔄

- 成果物: **`build/bin/Markmiru`**
- 固有オプション:
  - `-tags webkit2_41` … `libwebkit2gtk-4.1-dev` を使う環境向け

---

## ライセンス

Markmiru 本体および利用しているサードパーティライブラリのライセンスは [`LICENSE.md`](LICENSE.md) を確認してください。アプリ内では「ヘルプ → ライセンス...」から閲覧できます。
