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
- **再読み込み**（Ctrl+R。macOS は Cmd+R。メニュー「ファイル → 再読み込み」・右クリックメニューからも実行可）：表示中のタブのファイルを読み直し、他のプログラムで更新された内容を取り込む。Markmiru 上の未保存の変更がある場合は、変更を破棄して読み直すかキャンセルするかを確認する
- **ドラッグ&ドロップ**：Markdown ファイル（.md / .markdown / .mdown / .txt）をウィンドウに落として開く（複数可。最後に落としたファイルを表示）。既に開いているファイルは、そのタブに切り替えて再読み込みする
- **ページ内検索**（Ctrl+F。メニュー「検索...」は Windows/Linux、macOS は Cmd+F。閲覧・編集の両モード）
- **右クリックメニュー**（本文内。元に戻す／やり直し／切り取り／コピー／貼り付け／すべて選択／リンク操作。項目は常に同じ並びで、その場で使えないものはグレーアウト。右クリックは選択やカーソル位置を変えないため、貼り付けは直前のカーソル位置に入ります）
- **スタイル**による配色（ライト／ダーク）／表示のカスタマイズ（ライト／ダーク／GitHub 風／セピアのプリセットを複製して編集可）
  - 本文・見出し・コード・引用・リスト・表などの各パラメータを GUI 設定パネルで調整
  - カスタム CSS による上書き（上級者向け）
  - スタイルのエクスポート / インポート（JSON ファイル。別マシン・別 OS へ持ち運び可能）
- **PDF 出力 / 印刷**（OS / WebView の印刷機能。macOS はネイティブ印刷パネル。配色は印刷向けに変換して出力。ダーク系スタイルでも引用・表・リンク等を紙向けの配色で印刷）
  - 改ページ位置の明示的な指定（VS Code の Markdown PDF 拡張・Typora と同じ書き方。「[改ページの指定](#改ページの指定)」参照）
- **画像表示**：ローカルパス（相対・絶対・ルート相対）を data URI 化して表示。外部（リモート）画像はファイルごとに表示可否を確認
- **セキュリティ**：bluemonday によるサニタイズ、CSP 注入、外部リンクは確認ダイアログ後に OS ブラウザへ委譲
- **単一インスタンス**：多重起動を防止し、2 つ目の起動は既存ウィンドウへファイルを渡して前面化
- **セッション復元**：前回開いていたファイル群を起動時に再オープン（全件を起動時に読込。不在ファイルは 1 件ずつダイアログ）
- 同梱フォント（Noto Sans JP / Noto Serif JP / Noto Sans Mono）による OS 間の表示一致（等幅の日本語は Noto Sans JP へフォールバック）

## 改ページの指定

Markdown の記法には改ページがないため、印刷・PDF 出力で改ページしたい位置に、次のどちらかの HTML を書きます（**前後に空行を入れてください**）。

```html
<div class="page"></div>
```

```html
<div style="break-after: page"></div>
```

- 1 つ目は VS Code の Markdown PDF 拡張、2 つ目は Typora やブラウザ印刷全般と同じ書き方です。
- style で指定できるのは `break-after: page`・`break-before: page` と、旧表記の `page-break-after: always`・`page-break-before: always` です（`before` はその位置の前で改ページします）。それ以外の CSS は安全のため取り除かれます。
- 閲覧モードでは、改ページの位置を点線で示します（印刷・PDF には出ません）。
- 前後に空行が無いと、直後の行が Markdown として処理されず、書いた文字のまま表示されます。
- 文書の先頭に置くと 1 ページ目が空白に、2 つ続けて置くと間に空白ページが入ります（ブラウザの印刷と同じ挙動です）。
- **自己終了タグ `<div class="page"/>` には対応していません**（Markdown PDF 拡張では使えますが、Markmiru では改ページされず、点線が文書の末尾にまとめて表示されます）。`<div class="page"></div>` と書いてください。

## 依存パッケージ / ライブラリ

本アプリは **Wails v2 ＋ Go-SSR** 構成です。状態管理・Markdown 描画・HTML 生成を Go が担い（`html/template`）、htmx が操作を Go のエンドポイント（`http.Handler`）へ橋渡しします。WebView 内の自作 JavaScript は `glue.js` のみ（mermaid.js・htmx は既製ライブラリ）。フロントエンドのバンドル工程はありません。

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
| **macOS** | ✅ 対応・動作確認済み（Apple Silicon / Intel で実機検証済み）（WKWebView） |
| **Linux** | ✅ 対応・動作確認済み（amd64・Ubuntu 24.04 で実機検証済み）（WebKitGTK） |

- 最終目標は Windows / macOS / Linux のデスクトップ 3 種対応です。
- モバイル（iPhone / Android）は対象外です（Wails 採用のため）。
- **macOS は Apple Silicon（arm64）・Intel の両方、Linux は amd64（Ubuntu 24.04）で、それぞれビルド・起動・アプリ動作を実機で確認済みです。**Ubuntu 以外のディストリビューションでの動作は未確認です（必要パッケージは「[ビルド環境の構築手順](#ビルド環境の構築手順)」参照）。
- **右クリックメニューも Windows・macOS・Linux の 3 OS すべてで実機検証済み**です。

---

## 既知の制限事項

「軽量・閲覧重視」の方針や安全性の確保に基づく仕様上の制限です（プラットフォーム対応状況・配布形態は上記／下記を参照）。

- **別サーバーを指す UNC 画像は表示しません**：文書とは**別のサーバー**を指す UNC パス（`\\別ホスト\…`）の画像は、認証情報の外部漏洩（Windows の SMB 自動認証）を防ぐため表示しません（確認ダイアログも出ません）。文書と同じサーバー上の画像は表示できます。別サーバーの画像を使いたい場合は、相対パス化するか、ドライブレター（`Z:\…` 等、ネットワークドライブの割り当てを含む）で指定してください。
- **外部変更を自動では検知しません**：開いている最中に他のプログラムがそのファイルを書き換えても、自動再読み込みや警告は行いません。取り込むときは「**再読み込み**」（Ctrl+R）を使ってください（誤って古い内容で上書きしないよう、未変更のタブは保存時にファイルへ書き込みません）。
- **文字コードは UTF-8 のみ**：UTF-16 など他の文字コードのファイルは正しく表示・保存できません。BOM の有無と改行コード（CRLF / LF）は元のファイルのまま保存します（改行コードが混在する場合は数の多い方にそろえます）。Markmiru で新規作成したファイルは BOM なし・LF で保存します。
- **自動保存はありません**：保存は手動（Ctrl+S）です。アプリが異常終了した場合、その時点の未保存の変更は失われます。
- **編集モードは最小構成**（閲覧重視）：行番号・現在行ハイライト・複数カーソル／矩形選択・折りたたみ・入力補完・Lint はありません。
- **macOS の編集メニューは英語表記**（"Edit" / "Copy" 等）：Wails v2 のネイティブ実装の制約で日本語化できません（ファイル・表示・ヘルプ等は日本語）。
- **システムフォント選択時の表示差**：OS 間で同一の表示を保証するのは同梱フォントのみです。システムフォントを選ぶと OS により字形・字幅が異なる場合があります。

---

## インストール方法

> 配布は**インストーラ形式ではなく、ビルド成果物をまとめた簡素なアーカイブ**です（**Windows**: `.exe` のみを ZIP／**macOS**: `.app` ごと ZIP／**Linux**: バイナリのみを tar.gz）。[GitHub Releases](https://github.com/osakichi/Markmiru/releases) から入手できます。ソースからビルドする場合は「[ソースからのビルド手順](#ソースからのビルド手順)」を参照してください（ビルドスクリプトは `dist/` に配布用アーカイブを出力します）。

### Windows

配布 ZIP を展開した（またはビルドした）`Markmiru.exe` は単体で動作します。任意のフォルダに配置してそのまま実行できます。

- 動作には **WebView2 ランタイム**が必要です。Windows 10 / 11 には標準で同梱されていることがほとんどですが、未導入の場合は [Microsoft Edge WebView2 ランタイム](https://developer.microsoft.com/microsoft-edge/webview2/) を導入してください。

### macOS

- 配布 ZIP を展開した（またはビルドした）**`Markmiru.app`** を `/Applications`（アプリケーション）フォルダへドラッグして配置します。
- 署名・公証を行っていないため、初回起動時に Gatekeeper の警告が出る想定です。その場合は Finder でアプリを右クリック →「開く」で実行を許可するか、必要に応じて隔離属性を解除します:
  ```bash
  xattr -dr com.apple.quarantine /Applications/Markmiru.app
  ```

### Linux

- ビルドした実行ファイル **`build/bin/Markmiru`**（または配布 tar.gz を展開したバイナリ）を任意のディレクトリ（例: `~/.local/bin`）に配置して起動します:
  ```bash
  tar -xzf Markmiru-linux-<arch>-<sha>-<yyyymmdd>.tar.gz   # 配布アーカイブの場合
  ./Markmiru
  ```
- 実行には **WebKitGTK 4.1 / GTK3 のランタイム**が必要です（Ubuntu などデスクトップ環境には標準搭載。ビルド環境なら開発パッケージに含まれます）。
- 起動時に、ドック・Alt+Tab・アプリ一覧でアイコンを正しく表示するための**デスクトップエントリとアイコンをユーザーのデータディレクトリへ自動登録**します（GNOME 等のシェルはこれらを `.desktop` ファイル経由でしか解決しないため。登録に失敗してもアプリの動作には影響しません）:
  - `~/.local/share/applications/Markmiru.desktop` — 起動用エントリ。実行ファイルの場所を記録しており、バイナリを移動した場合も次回起動時に自動で追従します。副次効果として GNOME のアプリ一覧（アクティビティ）からも起動できるようになります。
  - `~/.local/share/icons/hicolor/512x512/apps/Markmiru.png` — アプリアイコン。
  - いずれも `XDG_DATA_HOME` を設定している場合はその配下に作成します。アンインストール時はバイナリと合わせてこの 2 ファイルを削除してください。

---

## ビルド環境の構築手順

Go・Wails CLI・git の導入は全 OS で共通です。これに加えて、OS ごとにネイティブ依存（WebView ランタイム・C コンパイラ等）の導入が必要です。本アプリは Go-SSR で**ビルドすべきフロントエンドのバンドルが無い**ため、フロントエンド用の追加ツールチェーンは不要です（ビルドは Go と Wails のみで完結します）。

### 共通手順

1. **Go（1.25 以上）** をインストール
   - 入手元: [https://go.dev/dl/](https://go.dev/dl/)（各 OS 用インストーラ／アーカイブ）。Homebrew・パッケージマネージャでも可。
   - 確認: `go version`
   - `go install` した実行ファイルは既定で `go\bin`（Windows: `%USERPROFILE%\go\bin` / macOS・Linux: `~/go/bin`）に置かれます。
2. **git** をインストール（確認: `git --version`）
   - ビルドスクリプト（`build.ps1` / `build.sh`）は**版として git のショート SHA を埋め込む**ため、ビルド時にも git が必要です。**git リポジトリの作業ツリー内**（`git clone` した状態）でビルドしてください。ソースを ZIP 等で展開しただけの非 git 環境では SHA 取得に失敗してビルドが停止します。その場合は版を埋め込まない**素の `wails build`**（バージョンは `dev`）を使用してください。
3. **Wails CLI** をインストール（コマンドは全 OS 同一）
   ```
   go install github.com/wailsapp/wails/v2/cmd/wails@latest
   ```
   - インストール先は上記 `go\bin`。`go\bin` を `PATH` に通すと `wails` だけで呼び出せます（PATH の通し方は後述の「OS 固有事項」を参照）。
4. **OS 固有のネイティブ依存**（後述）を導入してから、**環境チェック**を実行（コマンドは全 OS 同一）
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
- **SDK は macOS 11 以上が必要**です。Wails v2 が macOS 11 で追加された API（`UNNotificationPresentationOptionList` 等）を参照するため、Command Line Tools 付属の SDK が古いと `use of undeclared identifier 'UNNotificationPresentationOptionList'` でコンパイルに失敗します。SDK バージョンは `xcrun --show-sdk-version` で確認でき、古い場合は Command Line Tools を入れ直してください:
  ```bash
  sudo rm -rf /Library/Developer/CommandLineTools
  xcode-select --install
  ```
- **WKWebView** は macOS 標準のため追加導入は不要の想定です。
- **PATH の通し方**: `export PATH="$HOME/go/bin:$PATH"` を `~/.zshrc` 等に追記。

#### Linux

ディストリビューションにより必要パッケージ名は異なります。**C コンパイラ・GTK・WebKitGTK・pkg-config** を導入します。

- Debian / Ubuntu 系:
  ```bash
  sudo apt update
  sudo apt install build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev
  ```
  - WebKitGTK は **4.1 を推奨**します（Ubuntu 24.04 以降は 4.1 のみ提供）。4.1 環境でのビルドには `-tags webkit2_41` が必要ですが、**ビルドスクリプト（`build.sh`）は 4.1 の有無を自動判別して付与**します。4.1 が無い古い環境では `libwebkit2gtk-4.0-dev` を使用（タグ不要）。
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
   - macOS / Linux（macOS は Apple Silicon・Intel / Linux は amd64・Ubuntu 24.04 で動作確認済み）:
     ```bash
     ./scripts/build.sh
     ```
   - このスクリプトは **git のショート SHA をバージョンとして埋め込んだ上で内部的に `wails build` を実行**します（埋め込んだ版は「ヘルプ → Markmiru について」やファイルのプロパティ＝製品バージョンで確認できます）。
   - **バージョン埋め込みが不要なら、素の `wails build` でもビルドできます**（その場合バージョンは `dev` 表示になります）。`wails` を `PATH` に通していない場合はフルパスで呼び出します（例: Windows `& "$env:USERPROFILE\go\bin\wails.exe" build` / macOS・Linux `~/go/bin/wails build`）。Linux で WebKitGTK 4.1 のみの環境（Ubuntu 24.04 以降）では `-tags webkit2_41` を付与してください（ビルドスクリプト経由なら自動判別）。
   - ビルドスクリプトは `wails` を**既定のインストール先の固定パス**（Windows: `%USERPROFILE%\go\bin\wails.exe` / macOS・Linux: `~/go/bin/wails`）で呼び出します（`PATH` は参照しません）。`GOPATH` / `GOBIN` を変更して別の場所にインストールしている場合は、スクリプト内の `wails` のパスを環境に合わせて修正してください。
   - Go バインディング生成・Go のコンパイルは **Wails が自動で実行**します（フロントエンドの install/build は無いため Wails は `No Install command. Skipping.` / `No Build command. Skipping.` と表示してスキップします）。
   - 成果物は `build/bin/` に出力されます（ファイル名は OS により異なる。後述）。
   - 続けて**配布用アーカイブを `dist/` に出力**します（命名: `Markmiru-<platform>-<arch>-<sha>-<yyyymmdd>.zip`、Linux は `.tar.gz`。`<sha>` は上記バージョンと同じ git ショート SHA、`<yyyymmdd>` は作成日）。Windows は `.exe` のみ・macOS は `.app` ごとを ZIP 化、Linux はバイナリのみを tar.gz 化（実行権限を保持するため ZIP ではなく tar.gz）。配布方針は「[インストール方法](#インストール方法)」参照。
   - 初回ビルドは Go モジュールの取得が走るため時間がかかります（ネットワーク接続が必要）。2 回目以降はキャッシュにより短縮されます。

> **`-clean` / `-debug` / `-platform` / `-tags` 等の `wails build` オプション**を使う場合は、ビルドスクリプトを介さず `wails build <オプション>` を直接実行してください（その場合バージョンは `dev`）。バージョンも埋め込みたいときは `-ldflags "-X main.version=<任意>"` を併用します。

### OS 固有事項

#### Windows

- 成果物: **`build\bin\Markmiru.exe`**（配布は `dist/` の ZIP）
- **PowerShell の実行ポリシー**がスクリプト実行を禁止している環境（`Restricted`）では `build.ps1` を実行できません。`Get-ExecutionPolicy` で確認し、必要に応じて `Set-ExecutionPolicy -Scope CurrentUser RemoteSigned` を設定するか、`powershell -ExecutionPolicy Bypass -File .\scripts\build.ps1` で実行してください。
- 固有オプション:
  - `-debug` … デバッグ情報付き（開発者ツールを開けるビルド）

#### macOS

- 成果物: **`build/bin/Markmiru.app`**
- 固有オプション:
  - `-platform darwin/universal` … Apple Silicon ＋ Intel のユニバーサルバイナリを生成

#### Linux

- 成果物: **`build/bin/Markmiru`**（配布は `dist/` の tar.gz）
- 固有オプション:
  - `-tags webkit2_41` … `libwebkit2gtk-4.1-dev` を使う環境向け（素の `wails build` 実行時のみ必要。ビルドスクリプトは自動判別）

---

## ライセンス

Markmiru 本体および利用しているサードパーティライブラリのライセンスは [`LICENSE.md`](LICENSE.md) を確認してください。アプリ内では「ヘルプ → ライセンス...」から閲覧できます。
