#!/usr/bin/env bash
# Markmiru ビルドスクリプト（macOS / Linux）。
#
# 検証状況: macOS（arm64・x86_64）・Linux（amd64, Ubuntu 24.04）で動作確認済み
#   （いずれもビルド・起動・アプリ動作まで実機検証）。
#
# Go のみのビルド: 本アプリは Go-SSR（Wails AssetServer.Handler）で、install/bundle すべき
# フロントエンドは無い。wails build が Go バックエンドをコンパイルして exe を
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

# Linux: Wails v2 の既定リンク先は webkit2gtk-4.0 だが、Ubuntu 24.04 以降には 4.0 が無く
# 4.1 のみのため、4.1 がある環境では webkit2_41 タグでビルドする（無い環境はタグ無し＝4.0）。
build_tags=""
if [ "$(uname -s)" = "Linux" ] && pkg-config --exists webkit2gtk-4.1 2>/dev/null; then
  build_tags="webkit2_41"
fi

# -clean で build/bin を先に一掃し、古い実行バイナリ/成果物を残さない。
if [ -n "$build_tags" ]; then
  "$wails" build -clean -tags "$build_tags" -ldflags "-X main.version=$sha"
else
  "$wails" build -clean -ldflags "-X main.version=$sha"
fi

# 配布用アーカイブを dist/ に作成する（配布方針＝各 OS とも単純なアーカイブを配るだけ）。
# 名前: Markmiru-<platform>-<arch>-<sha>-<yyyymmdd>.zip（Linux は .tar.gz）
#   - <sha>      = 上のバージョンと同じ git ショート SHA（dirty 時は -dirty）
#   - <yyyymmdd> = このアーカイブを作成した日付
# dist/ は git 管理外（配布物はコミットしない）。ビルド成功時のみここへ到達する（set -e）。
date_stamp="$(date +%Y%m%d)"
dist_dir="$root/dist"
mkdir -p "$dist_dir"
os="$(uname -s)"
# <arch> は Go 表記（amd64 / arm64）で 3 OS 揃える。uname -m は表記が割れるため使わない
# （Linux の 64bit x86 は x86_64、ARM は aarch64。同じものを指すが Go 表記と一致しない）。
# go env GOARCH は GOARCH 環境変数があればそれ、無ければホストの値を返す。これは
# wails build がターゲットを決める規則（cmd/wails/flags/build.go）と同じため、名前が実体とずれない。
arch="$(go env GOARCH)"
[ -n "$arch" ] || { echo "error: go env GOARCH が空です" >&2; exit 1; }
if [ "$os" = "Darwin" ]; then
  # macOS: .app フォルダごと配布。ditto で固める（シンボリックリンク・実行権限を保持。
  # 素の zip や非 macOS 上での圧縮はバンドルを壊すため不可）。
  zip_path="$dist_dir/Markmiru-darwin-$arch-$sha-$date_stamp.zip"
  rm -f "$zip_path"
  ditto -c -k --keepParent "$root/build/bin/Markmiru.app" "$zip_path"
  echo "Packaged: $zip_path"
else
  # Linux: バイナリのみを tar.gz で配布（実行権限を確実に保持できるため ZIP ではなく tar.gz）。
  # AppImage 案は不採用——WebKitGTK は補助プロセス構成のため同梱が事実上困難で、巨大・脆弱になる
  # （検討経緯は docs/アーキテクチャ・画面設計.md §10）。実行には WebKitGTK 4.1 / GTK3
  # （Ubuntu デスクトップ標準搭載）が必要（README「インストール方法」参照）。
  tar_path="$dist_dir/Markmiru-linux-$arch-$sha-$date_stamp.tar.gz"
  rm -f "$tar_path"
  tar -C "$root/build/bin" -czf "$tar_path" Markmiru
  echo "Packaged: $tar_path"
fi
