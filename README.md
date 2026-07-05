# Markmiru

Markdown ドキュメントの**閲覧・編集**を行うデスクトップアプリ（**Markmiru** = Markdown ＋ 見る[miru]）。

## 概要

**Markmiru** は、Markdown を**素早く・簡単に**閲覧することに重点を置いたデスクトップアプリです。Markdown を綺麗にレンダリングして見る「**閲覧モード**」と、簡単な修正等のため Markdown 記法を直接編集する「**編集モード**」を備えています。

- mermaid 図・コードハイライト・GFM（表・チェックリスト等）に対応
- Wails v2（Go ＋ OS 標準 WebView）採用で**軽量・高速起動**を目指す
- プラグイン不要の**単体アプリ**として動作
- 信頼できない Markdown を開いても安全なよう**防御**  
  （HTML サニタイズ・CSP・外部リンクは確認後に OS ブラウザで開く・外部画像は確認後に表示）

## 機能一覧

- **マルチタブ**で複数ドキュメントを切り替えて表示
- **閲覧モード**：goldmark（Go）による GFM レンダリング表示
  - mermaid 図のレンダリング（`securityLevel: 'strict'`）
  - chroma（Go）によるコードシンタックスハイライト
  - GFM（表・チェックリスト・打消し線・自動リンク）・脚注
- **編集モード**：透明 textarea を chroma ハイライト層に重ねるオーバーレイ方式（ソフトラップ・控えめな構文ハイライト）
- 閲覧／編集モードの切り替え（セッション復元時は**常に閲覧モード**で開く）
- **ページ内検索**（Ctrl+F。メニュー「検索...」は Windows/Linux、macOS は Cmd+F。閲覧・編集の両モード）
- **スタイル**による配色（ライト／ダーク）／表示のカスタマイズ（ライト／ダーク／GitHub 風／セピアのプリセットを複製して編集可）
  - 本文・見出し・コード・引用・リスト・表などの各パラメータを GUI 設定パネルで調整
  - カスタム CSS による上書き（上級者向け）
  - スタイルのエクスポート / インポート（JSON ファイル。別マシン・別 OS へ持ち運び可能）
- **PDF 出力 / 印刷**（WebView の印刷機能。配色は印刷向けに変換して出力）
- **画像表示**：ローカルパス（相対・絶対・ルート相対）を data URI 化して表示。外部（リモート）画像はファイルごとに表示可否を確認
- **セキュリティ**：bluemonday によるサニタイズ、CSP 注入、外部リンクは確認ダイアログ後に OS ブラウザへ委譲
- **単一インスタンス**：多重起動を防止し、2 つ目の起動は既存ウィンドウへファイルを渡して前面化
- **セッション復元**：前回開いていたファイル群を起動時に再オープン（全件を起動時に読込。不在ファイルは 1 件ずつダイアログ）
- 同梱フォント（Noto Sans JP / Noto Serif JP / Noto Sans Mono）による OS 間の表示一致（等幅の日本語は Noto Sans JP へフォールバック）

## 依存パッケージ / ライブラリ

本アプリは **Wails v2 ＋ Go-SSR** 構成です。状態管理・Markdown 描画・HTML 生成を Go が担い（`html/template`）、htmx が操作を Go のエンドポイント（`http.Handler`）へ橋渡しします。WebView 内の自作 JavaScript は `glue.js` のみ（mermaid.js・htmx は既製ライブラリ）。Node/npm/Vite は使用しません。

### バックエンド（Go）

| ライブラリ | 用途 |
|------------|------|
| [Wails v2](https://wails.io/)（`github.com/wailsapp/wails/v2`） | アプリ基盤（Go ＋ OS 標準 WebView）・`AssetServer.Handler` による Go-SSR |
| [goldmark](https://github.com/yuin/goldmark) | Markdown（CommonMark ＋ GFM・脚注）解析・HTML 化 |
| [Chroma](https://github.com/alecthomas/chroma) | コードシンタックスハイライト（CSS クラス出力） |
| [bluemonday](https://github.com/microcosm-cc/bluemonday) | レンダリング HTML のサニタイズ |
| `golang.org/x/net` / `golang.org/x/sys` | HTML 解析補助（サニタイズ）・OS 依存処理（単一インスタンス IPC のピア検証） |
| Go 標準ライブラリ（`html/template` / `net/http` / `os` / `encoding/json` / `path/filepath` 等） | HTML 生成・htmx エンドポイント・ファイル I/O・設定永続化 |

### WebView（表示層）

| ライブラリ | 用途 |
|------------|------|
| [htmx](https://htmx.org/) | 操作 → Go へ要求 → HTML 断片で DOM 差し替え |
| [mermaid](https://mermaid.js.org/) | 図表のレンダリング（`securityLevel: 'strict'`） |
| `glue.js`（自作） | 編集オーバーレイのスクロール同期・ページ内検索・外部リンク制御・イベントブリッジ等の最小グルー |
| Noto Sans JP / Noto Serif JP / Noto Sans Mono（woff2 を直 vendor） | 同梱フォント（自己ホスト。OS 間の表示一致） |

> 正確なバージョンは [`go.mod`](go.mod) を参照してください。ライセンス表記は [`LICENSE.md`](LICENSE.md) に統合しています。

## 対応プラットフォーム

| プラットフォーム | 状態 |
|------------------|------|
| **Windows** | ✅ 対応・動作確認済み（WebView2） |
| **macOS** | ✅ 対応（arm64 で実機検証済み。ただし Go-SSR 化後の再検証は未実施）（WKWebView） |
| **Linux** | 🔄 コード対応済み・**未検証**（WebKitGTK） |

- 最終目標は Windows / macOS / Linux のデスクトップ 3 種対応です。
- モバイル（iPhone / Android）は対象外です（Wails 採用のため）。
- **macOS は arm64 でビルド・起動・アプリ機能まで実機で確認済みです（Go-SSR 化後の再検証は未実施）。Linux の手順は Wails の標準的な要件に基づく想定で、実機での確認は行っていません**（今後整備予定）。以降の各節で Linux はこの前提です（🔄 印で示します）。

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

Go・Wails CLI の導入は全 OS で共通です。これに加えて、OS ごとにネイティブ依存（WebView ランタイム・C コンパイラ等）の導入が必要です。**Node.js / npm / Vite は不要**です（本アプリは Go-SSR で、ビルドすべきフロントエンドのバンドルがありません）。

### 共通手順

1. **Go（1.25 以上）** をインストール
   - 入手元: [https://go.dev/dl/](https://go.dev/dl/)（各 OS 用インストーラ／アーカイブ）。Homebrew・パッケージマネージャでも可。
   - 確認: `go version`
   - `go install` した実行ファイルは既定で `go\bin`（Windows: `%USERPROFILE%\go\bin` / macOS・Linux: `~/go/bin`）に置かれます。
2. **Wails CLI** をインストール（コマンドは全 OS 同一）
   ```
   go install github.com/wailsapp/wails/v2/cmd/wails@latest
   ```
   - インストール先は上記 `go\bin`。`go\bin` を `PATH` に通すと `wails` だけで呼び出せます（PATH の通し方は後述の「OS 固有事項」を参照）。
3. **OS 固有のネイティブ依存**（後述）を導入してから、**環境チェック**を実行（コマンドは全 OS 同一）
   ```
   wails doctor
   ```
   - Go / WebView ランタイム / C コンパイラ等の状態が一覧表示されます。不足や警告が出た項目は指示に従って導入・修正してください。すべて「OK」になればビルド可能です。

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
   - macOS / Linux（macOS は arm64 で動作確認済み〔Go-SSR 化後は再検証待ち〕 / Linux は未検証 🔄）:
     ```bash
     ./scripts/build.sh
     ```
   - このスクリプトは **git のショート SHA をバージョンとして埋め込んだ上で内部的に `wails build` を実行**します（埋め込んだ版は「ヘルプ → Markmiru について」やファイルのプロパティ＝製品バージョンで確認できます）。
   - **バージョン埋め込みが不要なら、素の `wails build` でもビルドできます**（その場合バージョンは `dev` 表示になります）。`wails` を `PATH` に通していない場合はフルパスで呼び出します（例: Windows `& "$env:USERPROFILE\go\bin\wails.exe" build` / macOS・Linux `~/go/bin/wails build`）。
   - Go バインディング生成・Go のコンパイルは **Wails が自動で実行**します（フロントエンドの install/build は無いため Wails は `No Install/Build command. Skipping.` と表示してスキップします）。
   - 成果物は `build/bin/` に出力されます（ファイル名は OS により異なる。後述）。
   - 続けて**配布用アーカイブを `dist/` に出力**します（命名: `Markmiru-<platform>-<arch>-<sha>-<yyyymmdd>.zip`。`<sha>` は上記バージョンと同じ git ショート SHA、`<yyyymmdd>` は作成日）。Windows は `.exe` のみ・macOS は `.app` ごとを ZIP 化。**Linux 用アーカイブは将来対応予定**で現状は作成しません（バイナリは `build/bin/` に残ります）。配布方針は「[インストール方法](#インストール方法)」参照。
   - 初回ビルドは Go モジュールの取得が走るため時間がかかります（ネットワーク接続が必要）。2 回目以降はキャッシュにより短縮されます。

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
