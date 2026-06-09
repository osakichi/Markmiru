# Markmiru

Markdown ドキュメントの**閲覧・編集**を行うデスクトップアプリ（**Markmiru** = Markdown ＋ 見る）。

## 概要

**Markmiru** は、Markdown を**素早く・簡単に**閲覧することに重点を置いたデスクトップアプリです。Markdown を綺麗にレンダリングして見る「**閲覧モード**」と、簡単な修正等のため Markdown 記法を直接編集する「**編集モード**」を備えています。

- mermaid 図・コードハイライト・GFM（表・チェックリスト等）に対応
- Wails v2（Go ＋ OS 標準 WebView）採用で、Electron に比べ**軽量・高速起動**
- プラグイン不要の**単体アプリ**として動作
- 信頼できない Markdown を開いても安全なよう**防御**（HTML サニタイズ・CSP・外部リンクは OS ブラウザへ委譲・外部画像はファイルごとに表示確認）

## 機能一覧

- **マルチタブ**で複数ドキュメントを切り替えて表示
- **閲覧モード**：markdown-it によるレンダリング表示
  - mermaid 図のレンダリング（`securityLevel: 'strict'`）
  - highlight.js によるコードシンタックスハイライト
  - GFM（表・チェックリスト・打消し線・自動リンク）
- **編集モード**：CodeMirror 6（行番号・ソフトラップ・控えめな構文ハイライト）
- 閲覧／編集モードの切り替え（セッション復元時は**常に閲覧モード**で開く）
- **スタイルプロファイル**によるテーマ／表示カスタマイズ（ライト／ダーク／GitHub 風／セピア、複製して編集可）
  - 本文・見出し・コード・引用・リスト・表などの各パラメータを GUI 設定パネルで調整
  - カスタム CSS による上書き（上級者向け）
- **PDF 出力 / 印刷**（WebView の印刷機能。既定は印刷向け配色に変換、「表示通りに印刷」で画面通り）
- **画像表示**：ローカルパス（相対・絶対・ルート相対）を data URI 化して表示。外部（リモート）画像はファイルごとに表示可否を確認
- **セキュリティ**：DOMPurify によるサニタイズ、本番ビルドでの CSP 注入、外部リンクの OS ブラウザ委譲
- **単一インスタンス**：多重起動を防止し、2 つ目の起動は既存ウィンドウへファイルを渡して前面化
- **セッション復元**：前回開いていたファイル群を起動時に再オープン（遅延読込。不在ファイルは 1 件ずつダイアログ）
- 同梱フォント（Noto Sans / Serif / Mono JP）による OS 間の表示一致

## 依存パッケージ / ライブラリ

### バックエンド（Go）

| ライブラリ | 用途 |
|------------|------|
| [Wails v2](https://wails.io/)（`github.com/wailsapp/wails/v2`） | アプリ基盤（Go ＋ OS 標準 WebView） |
| `golang.org/x/sys` | OS 依存処理 |
| Go 標準ライブラリ（`os` / `encoding/json` / `path/filepath` 等） | ファイル I/O・設定永続化 |

### フロントエンド（Svelte / WebView）

| ライブラリ | 用途 |
|------------|------|
| [Svelte](https://svelte.dev/) ＋ [Vite](https://vitejs.dev/) ＋ TypeScript | UI フレームワーク・ビルド |
| [markdown-it](https://github.com/markdown-it/markdown-it) ＋ markdown-it-task-lists | Markdown 解析 |
| [mermaid](https://mermaid.js.org/) | 図表のレンダリング |
| [highlight.js](https://highlightjs.org/) | コードシンタックスハイライト |
| [CodeMirror 6](https://codemirror.net/)（`codemirror` / `@codemirror/lang-markdown` / `@codemirror/state` / `@codemirror/theme-one-dark`） | 編集モードのエディタ |
| [DOMPurify](https://github.com/cure53/DOMPurify) | レンダリング HTML のサニタイズ |
| [@fontsource](https://fontsource.org/) Noto Sans JP / Noto Serif JP / Noto Sans Mono | 同梱フォント（自己ホスト） |

> 正確なバージョンは [`go.mod`](go.mod) と [`frontend/package.json`](frontend/package.json) を参照してください。ライセンス表記は [`LICENSE.md`](LICENSE.md) に統合しています。

## 対応プラットフォーム

| プラットフォーム | 状態 |
|------------------|------|
| **Windows** | ✅ 対応・動作確認済み（WebView2） |
| **macOS** | 🔄 コード対応済み・**未検証**（WKWebView） |
| **Linux** | 🔄 コード対応済み・**未検証**（WebKitGTK） |

- 最終目標は Windows / macOS / Linux のデスクトップ 3 種対応です。
- モバイル（iPhone / Android）は対象外です（Wails 採用のため）。
- **macOS / Linux の手順は Wails の標準的な要件に基づく想定であり、実機での確認は行っていません**（今後整備予定）。以降の各節で macOS / Linux はこの前提です（🔄 印で示します）。

---

## インストール方法

> ⚠️ 現時点ではいずれのプラットフォームも配布用インストーラ／パッケージは未提供です。各 OS とも下記「[ソースからのビルド手順](#ソースからのビルド手順手作業)」でビルドした実行ファイルを利用してください（インストーラ／パッケージ形式での配布は今後対応予定）。

### Windows

ビルド済みの `build\bin\Markmiru.exe` は単体で動作します。任意のフォルダに配置してそのまま実行できます。

- 動作には **WebView2 ランタイム**が必要です。Windows 10 / 11 には標準で同梱されていることがほとんどですが、未導入の場合は [Microsoft Edge WebView2 ランタイム](https://developer.microsoft.com/microsoft-edge/webview2/) を導入してください。

### macOS 🔄

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

---

## ビルド環境の構築手順（手作業）

Go・Node.js・Wails CLI の導入は全 OS で共通です。これに加えて、OS ごとにネイティブ依存（WebView ランタイム・C コンパイラ等）の導入が必要です。

### 共通手順

1. **Go（1.23 以上）** をインストール
   - 入手元: [https://go.dev/dl/](https://go.dev/dl/)（各 OS 用インストーラ／アーカイブ）。Homebrew・パッケージマネージャでも可。
   - 確認: `go version`
   - `go install` した実行ファイルは既定で `go\bin`（Windows: `%USERPROFILE%\go\bin` / macOS・Linux: `~/go/bin`）に置かれます。
2. **Node.js（LTS 推奨）＋ npm** をインストール
   - 入手元: [https://nodejs.org/](https://nodejs.org/)（npm 同梱）。Homebrew・パッケージマネージャ・[nvm](https://github.com/nvm-sh/nvm) でも可。
   - 確認: `node -v` / `npm -v`
   - フロントエンドの依存取得（`npm install`）はビルド時に Wails が自動実行するため、手動での `npm install` は通常不要です。
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

#### macOS 🔄

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

## ソースからのビルド手順（手作業）

`git clone` と `wails build` の中核は全 OS 共通です。成果物のパスと一部のオプションのみ OS で異なります。

### 共通手順

1. リポジトリを取得
   ```
   git clone <このリポジトリの URL> Markmiru
   cd Markmiru
   ```
2. 本番ビルド
   ```
   wails build
   ```
   - フロントエンドの依存取得（`npm install`）・ビルド（`npm run build`）・Go バインディング生成は **Wails が自動で実行**します。
   - 成果物は `build/bin/` に出力されます（ファイル名は OS により異なる。後述）。
   - 共通オプション: `-clean`（`build/bin` をクリーンしてからビルド）。
   - 初回ビルドは Go モジュール・npm パッケージの取得が走るため時間がかかります（ネットワーク接続が必要）。2 回目以降はキャッシュにより短縮されます。

> `wails` を `PATH` に通していない場合は、フルパスで呼び出します（例: Windows `& "$env:USERPROFILE\go\bin\wails.exe" build` / macOS・Linux `~/go/bin/wails build`）。

### OS 固有事項

#### Windows

- 成果物: **`build\bin\Markmiru.exe`**
- 固有オプション:
  - `-debug` … デバッグ情報付き（開発者ツールを開けるビルド）
  - `-nsis` … NSIS インストーラを生成（要 [NSIS](https://nsis.sourceforge.io/)。配布用パッケージを作る場合）

#### macOS 🔄

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
