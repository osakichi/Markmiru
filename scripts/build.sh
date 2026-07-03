#!/usr/bin/env bash
# Markmiru ビルドスクリプト（macOS / Linux）。
#
# 検証状況: macOS（arm64）で動作確認済み（ビルド・起動・アプリ機能まで実機検証）。Linux は未検証のため、
#   Linux 対応に着手する際に、このスクリプトの動作を必ず検証すること。
#
# Go のみのビルド: 本アプリは Go-SSR（Wails AssetServer.Handler）で、install/bundle すべき
# Node/npm/Vite のフロントエンドは無い。wails build が Go バックエンドをコンパイルして exe を
# パッケージし、-clean が build/bin の旧成果物を一掃する。Go は内容ハッシュのキャッシュでソース追従。
#
# git のショート SHA をバージョンとして埋め込む（Windows 版 build.ps1 と同等）:
#   - 実行時（メニュー「Markmiru について...」の末尾に表示）: Go の ldflags (-X main.version=<sha>)
#   - OS のプロパティ（macOS: 情報を見る → バージョン）: wails.json の info.productVersion に一時注入。
# 注入した wails.json はビルド後に必ず元へ戻す（リポジトリに SHA の差分を残さない）。
#
# 使い方: リポジトリ直下で  ./scripts/build.sh
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
wails_json="$root/wails.json"
wails="$HOME/go/bin/wails"

# 版＝git ショート SHA。作業ツリーが汚れていれば -dirty を付与。
sha="$(git -C "$root" rev-parse --short HEAD)"
if [ -n "$(git -C "$root" status --porcelain)" ]; then
  sha="$sha-dirty"
fi
echo "Markmiru version: $sha"

# wails.json を退避し、終了時（成功・失敗問わず）に必ず復元する。
# 復元時は productVersion を常に "dev" へ戻す（前回クラッシュで SHA が残っていても自己修復。
# 他の編集は退避内容から保持する）。
backup="$(mktemp)"
cp "$wails_json" "$backup"
trap 'sed -E "s/(\"productVersion\"[[:space:]]*:[[:space:]]*)\"[^\"]*\"/\1\"dev\"/" "$backup" > "$wails_json"; rm -f "$backup"' EXIT

# info.productVersion の値だけを SHA に差し替える（sed -i 非依存のため一時ファイル経由）。
sed -E 's/("productVersion"[[:space:]]*:[[:space:]]*)"[^"]*"/\1"'"$sha"'"/' "$wails_json" > "$wails_json.tmp"
mv "$wails_json.tmp" "$wails_json"

# -clean で build/bin を先に一掃し、古い実行バイナリ/成果物を残さない。
"$wails" build -clean -ldflags "-X main.version=$sha"

# 配布用 ZIP を dist/ に作成する（配布方針＝各 OS とも単純な ZIP を配るだけ）。
# 名前: Markmiru-<platform>-<arch>-<sha>-<yyyymmdd>.zip
#   - <sha>      = 上のバージョンと同じ git ショート SHA（dirty 時は -dirty）
#   - <yyyymmdd> = この ZIP を作成した日付
# dist/ は git 管理外（配布物はコミットしない）。ビルド成功時のみここへ到達する（set -e）。
date_stamp="$(date +%Y%m%d)"
dist_dir="$root/dist"
mkdir -p "$dist_dir"
os="$(uname -s)"
arch="$(uname -m)"
if [ "$os" = "Darwin" ]; then
  # macOS: .app フォルダごと配布。ditto で固める（シンボリックリンク・実行権限を保持。
  # 素の zip や非 macOS 上での圧縮はバンドルを壊すため不可）。
  zip_path="$dist_dir/Markmiru-darwin-$arch-$sha-$date_stamp.zip"
  rm -f "$zip_path"
  ditto -c -k --keepParent "$root/build/bin/Markmiru.app" "$zip_path"
  echo "Packaged: $zip_path"
else
  # Linux: 配布方式は AppImage を「仮決め」（真の自己完結。WebKitGTK 等を同梱）。
  # 最終決定は Linux 対応に着手する際に行う。AppImage 生成（linuxdeploy 等）は未実装・未検証のため、
  # 現時点では ZIP を作成しない（バイナリは build/bin/Markmiru に残る）。
  echo "Linux packaging is tentatively AppImage and not yet implemented; skipping ZIP. (Linux is unverified.)"
fi
