// Markmiru グルー JS（Go-SSR）。
//
// WebView 内に残る唯一の自作 JS。役割は「仕組み的グルー」のみで、アプリロジックは持たない:
//   1. mermaid 再描画（初期表示＋htmx の本文差し替え後）
//   2. 編集オーバーレイのスクロール同期
//   3. ネイティブメニュー / IPC の Wails イベント → htmx.ajax ブリッジ
//   4. ダイアログのフォーカス付与・Esc=拒否・危険ダイアログの 500ms 活性化
//   5. 印刷トリガ（HX-Trigger: do-print → Go の Print）
//   6. ページ内検索（Ctrl+F）: 可視テキスト層の走査＋CSS Custom Highlight API
//   7. 外部リンクの遷移制御（WebView 遷移防止・確認ダイアログ・OS ブラウザ委譲）
//   8. 設定パネルの対入力同期（スライダー↔数値 / カラー↔HEX）
//   9. 右クリックメニュー（本文のみ。サーバ断片の注入・位置決め・アクション実行・クローズ）
//
// 設計: docs/アーキテクチャ・画面設計.md §1.2, §4.2, §4.4。CSP（script-src 'self'）下で動くよう外部ファイルに集約。
// Wails ランタイム（window.runtime / window.go）はシェル（ルート /）にのみ注入される。
(function () {
  'use strict'

  // --- 1. mermaid 描画 ---------------------------------------------------
  function mermaidTheme() {
    // 本文ラッパ（再描画ごとに最新の data-scheme が付く）を優先、無ければ #app。
    var el = document.querySelector('.preview-scroll[data-scheme]') || document.getElementById('app')
    return el && el.dataset.scheme === 'dark' ? 'dark' : 'default'
  }

  var currentTheme = ''
  function ensureMermaid(theme) {
    if (!window.mermaid || currentTheme === theme) return
    window.mermaid.initialize({ startOnLoad: false, theme: theme, securityLevel: 'strict' })
    currentTheme = theme
  }

  // 図の ID は mermaid.run() に任せず自前で採番する。run() の採番は非決定モード（既定）だと
  // `Date.now()` を返すだけで（mermaid 11.15.0 の InitIDGenerator）、**同じミリ秒に描画された図
  // どうしで ID が衝突する**。mermaid は描画中の一時要素を `"d" + ID` という id で document.body
  // 直下に置き `select("#d" + ID)` で引き当てるため、衝突すると別の図の一時要素を掴み、
  // 描画結果が別の図の枠に入る（＝図がずれる。2026-09-03 に Windows で再現）。
  // 連番を足せば同一ミリ秒でも一意になり、複数の run が重なっても衝突しない。
  var mermaidSeq = 0
  var mermaidChain = Promise.resolve() // 直列化用のチェーン（下の queueMermaid が繋いでいく）
  function runMermaid() {
    if (!window.mermaid) return
    var nodes = Array.prototype.slice.call(
      document.querySelectorAll('pre.mermaid:not([data-processed])')
    )
    if (!nodes.length) return
    ensureMermaid(mermaidTheme())
    for (var i = 0; i < nodes.length; i++) queueMermaid(nodes[i])
  }

  // queueMermaid は 1 図を描画キューへ積む。描画は**直列**（前の図の完了後に次を描く）で、
  // run() が図ごとに await するのと同じ順序性を保つ——並行に投げても ID は衝突しないが、
  // mermaid 内部の共有状態に依存する部分で挙動が変わり得るため、上流の実行順に合わせる。
  // data-processed はキューへ積む時点で付ける（run() が await の前に付けるのと同じ）。
  // 描画完了を待たずに付くので、重なって走る runMermaid が同じ図を二重に積むことはない。
  // 末尾の catch は保険。チェーンが reject のまま残ると以降の .then が実行されず、
  // 「それ以降どの図も描画されない」状態が続いてしまうため、必ず解決状態へ戻す。
  function queueMermaid(node) {
    node.setAttribute('data-processed', 'true')
    mermaidChain = mermaidChain
      .then(function () {
        return renderMermaid(node)
      })
      .catch(function (e) {
        console.error('mermaid queue error:', e)
      })
  }

  // renderMermaid は 1 図を描画して枠へ差し込む（run() が内部で行うのと同じ手順）。
  // 1 図の失敗で後続を止めないよう、失敗は握りつぶしてログのみ出す（run() と同じ扱い）。
  function renderMermaid(node) {
    var src = node.textContent // サーバはエスケープ済み。textContent で元の記述に戻る
    var id = 'mermaid-' + Date.now() + '-' + mermaidSeq++
    try {
      // 第 3 引数に枠を渡すのは run() と同じ（本文の CSS 下で文字幅を測らせ、レイアウトを揃える）。
      var p = window.mermaid.render(id, src, node)
      if (!p || !p.then) return
      return p
        .then(function (out) {
          node.innerHTML = out.svg
          if (out.bindFunctions) out.bindFunctions(node)
        })
        .catch(function (e) {
          console.error('mermaid render error:', e)
        })
    } catch (e) {
      console.error('mermaid render error:', e)
    }
  }

  // --- 4. ダイアログ: フォーカス / キージャック対策 ----------------------
  function activeDialog() {
    var host = document.getElementById('dialog-host')
    return host ? host.querySelector('.mm-dialog') : null
  }
  function setupDialog() {
    var dlg = activeDialog()
    if (!dlg || dlg.dataset.mmReady) return
    dlg.dataset.mmReady = '1'
    // WebView にキーボードフォーカスを与える（Windows/WebView2 対策。未提供なら無視）。
    try {
      window.go.main.App.FocusWindow()
    } catch (e) {
      /* バインディング未提供 */
    }
    // キージャック対策: 受諾ボタンはサーバ HTML で disabled 済み。500ms 後に有効化する。
    var accept = dlg.querySelector('[data-dialog-accept]')
    if (accept) {
      setTimeout(function () {
        accept.disabled = false
      }, 500)
    }
    // フォーカスはダイアログ本体へ（ボタンに当てず Enter での誤受諾を防ぐ）。
    dlg.setAttribute('tabindex', '-1')
    dlg.focus()
  }
  document.addEventListener('keydown', function (e) {
    // Ctrl+F / Cmd+F: ページ内検索を開く（メニューの "Ctrl+F" は表示専用のためここで拾う）。
    if ((e.ctrlKey || e.metaKey) && !e.shiftKey && !e.altKey && (e.key === 'f' || e.key === 'F')) {
      e.preventDefault()
      openFind()
      return
    }
    // Ctrl+S / Cmd+S: 「保存」メニューが無効の間（サーバがクリーン認識＝デバウンス未着を含む）
    // はネイティブアクセラレータが発火せず、キーが WebView まで届く。ここで拾って同じ保存
    // フロー（未送信の編集を flush してから保存）へ流す。メニューが有効なときはネイティブ側が
    // 先にキーを消費するため二重には走らない。
    if ((e.ctrlKey || e.metaKey) && !e.shiftKey && !e.altKey && (e.key === 's' || e.key === 'S')) {
      e.preventDefault()
      saveActive('/active/save')
      return
    }
    // Ctrl+A / Cmd+A: 入力欄の外では現在モードの本文だけを全選択し、ウィンドウ全体の選択を防ぐ。
    // メニュー「すべて選択」と同じ editExec に集約（編集→textarea 全選択 / 閲覧→本文のみ）。
    if ((e.ctrlKey || e.metaKey) && !e.shiftKey && !e.altKey && (e.key === 'a' || e.key === 'A')) {
      var ae = document.activeElement
      var tag = ae && ae.tagName
      if (tag === 'INPUT' || tag === 'TEXTAREA') return // 入力欄内はその欄を全選択（既定に任せる）
      e.preventDefault() // サイドバー/タブバー等にフォーカスがあってもウィンドウ全体選択を防ぐ
      editExec('selectAll')
      return
    }
    // Ctrl+Z / Ctrl+Shift+Z（Cmd+Z / Cmd+Shift+Z）: 取り消し / やり直し。
    // Linux（WebKitGTK）は GTK3 由来のキーバインドに取り消し／やり直しが無く、キーが編集コマンドへ
    // 変換されないため何も起きない。ここで拾ってメニューと同じ editExec（execCommand）へ流す。
    // OS 分岐は持たない: Windows はネイティブでも動くが、メニュー「取り消し」が通るのと同じ
    // execCommand 経路に合流するだけで結果は変わらない。macOS はネイティブ編集メニューのキー等価
    // （Cmd+Z / Cmd+Shift+Z）が WebView へ届く前に消費するため、ここは通らず従来どおり。
    // やり直しに Ctrl+Y を割り当てないのは、macOS では Ctrl+Y が入力欄の yank（AppKit の
    // 標準キーバインド）で、横取りすると OS 既定の動作を壊すため（メニュー表記も Ctrl+Shift+Z）。
    if ((e.ctrlKey || e.metaKey) && !e.altKey && (e.key === 'z' || e.key === 'Z')) {
      // IME 変換中は変換の取り消しを奪わない。本文以外の入力欄（検索バー・設定）はその欄自身の
      // 取り消しに任せる（Ctrl+A と同じ考え方）。
      if (e.isComposing) return
      var el = document.activeElement
      var elTag = el && el.tagName
      if ((elTag === 'INPUT' || elTag === 'TEXTAREA') && !(el.classList && el.classList.contains('editor-input'))) return
      e.preventDefault()
      editExec(e.shiftKey ? 'redo' : 'undo')
      return
    }
    // Enter: ダイアログに既定ボタン（安全な既定動作。例: 保存して閉じる）があれば実行する。
    // 危険な承諾ボタン（外部画像の「表示する」等）には既定を付けないため Enter では発火しない。
    if (e.key === 'Enter') {
      var d = activeDialog()
      if (!d) return
      var def = d.querySelector('[data-dialog-default]')
      if (def && !def.disabled) {
        e.preventDefault()
        def.click()
      }
      return
    }
    if (e.key !== 'Escape') return
    // Esc の優先順位: 右クリックメニュー → 確認ダイアログ拒否 → 設定モーダル → 検索バー。
    if (ctxMenu()) {
      e.preventDefault()
      closeCtxMenu()
      return
    }
    var dlg = activeDialog()
    if (dlg) {
      var reject = dlg.querySelector('[data-dialog-reject]')
      if (reject) {
        e.preventDefault()
        reject.click()
      }
      return
    }
    var sc = settingsCloseBtn()
    if (sc) {
      e.preventDefault()
      sc.click()
      return
    }
    if (findBar()) {
      e.preventDefault()
      closeFind()
    }
  })

  // --- 設定モーダル: Esc / 背景クリックで閉じる（開いているときのみ） ------
  function settingsOpen() {
    var s = document.getElementById('settings')
    return !!s && !s.hasAttribute('hidden')
  }
  function settingsCloseBtn() {
    return settingsOpen() ? document.querySelector('#settings [data-settings-close]') : null
  }
  document.addEventListener('click', function (e) {
    if (e.target && e.target.classList && e.target.classList.contains('mm-settings-backdrop')) {
      var sc = settingsCloseBtn()
      if (sc) sc.click() // 背景（ダイアログ外）クリックで閉じる
    }
  })

  // 初期表示＋htmx の差し替え/OOB 反映後の処理。afterSettle は OOB（#dialog-host 等）反映後に発火。
  function onUpdate() {
    runMermaid()
    setupDialog()
    setupFindBar()
    refreshFindIfOpen()
  }
  if (document.readyState !== 'loading') onUpdate()
  else document.addEventListener('DOMContentLoaded', onUpdate)
  document.body.addEventListener('htmx:afterSettle', onUpdate)
  // ダイアログは OOB で #dialog-host ごと差し替わり htmx イベントのタイミングが読みにくいため、
  // DOM 変化を監視して確実に初期化する（mmReady で二重処理を防止）。
  if (window.MutationObserver) {
    new MutationObserver(setupDialog).observe(document.body, { childList: true, subtree: true })
  }

  // --- 2. 編集オーバーレイ: スクロール同期 --------------------------------
  // textarea のスクロールをハイライト層（.hl）へ反映（scroll はバブルしないので capture）。
  // 入力ごとの再ハイライトは #hl の「中身だけ」を innerHTML で差し替えるため（fragments.html
  // の edit-sync）、<pre> 要素とそのスクロール位置は原則ブラウザが保持する。
  document.addEventListener(
    'scroll',
    function (e) {
      var ta = e.target
      if (ta && ta.classList && ta.classList.contains('editor-input')) {
        var hl = ta.parentElement && ta.parentElement.querySelector('.hl')
        if (hl) {
          hl.scrollTop = ta.scrollTop
          hl.scrollLeft = ta.scrollLeft
        }
      }
    },
    true
  )
  // ただし最下部付近で行数が増える編集（改行など）では、textarea が先に伸びて自動スクロール
  // する一方、ハイライト層は差し替え（150ms デバウンス）までの間 1 行短く、上の同期が上限で
  // クランプされて 1 行ズレた値のまま固定され得る（差し替え後は scroll イベントが来ない）。
  // そのため差し替え直後（afterSwap）に、対象がハイライト層のときだけ再同期する。
  document.body.addEventListener('htmx:afterSwap', function (e) {
    var hl = e.target
    if (hl && hl.classList && hl.classList.contains('hl')) {
      var ta = hl.parentElement && hl.parentElement.querySelector('.editor-input')
      if (ta) {
        hl.scrollTop = ta.scrollTop
        hl.scrollLeft = ta.scrollLeft
      }
    }
  })

  // --- 3. ネイティブメニュー / IPC → htmx ブリッジ ------------------------
  // メニュークリック/アクセラレータ（Ctrl+S 等）は Go が menu:* を発火する。
  // それを画面操作と同じ Go-SSR エンドポイントへ橋渡しする。
  function ajax(method, path, target, swap) {
    if (window.htmx) window.htmx.ajax(method, path, { target: target, swap: swap || 'innerHTML' })
  }
  // 保存の前処理: 編集中の textarea には未送信の内容（150ms デバウンス待ち）が残り得る。
  // そのまま保存するとサーバは古い内容を書き込む（クリーン認識なら no-op にすらなる）ため、
  // 保存系は必ず現在の内容を先にサーバへ同期（flush）してから保存エンドポイントを叩く。
  function saveActive(path) {
    var ta = document.querySelector('.editor-input')
    if (ta && window.htmx) {
      window.htmx
        .ajax('POST', ta.getAttribute('hx-post'), { source: ta, values: { value: ta.value } })
        .then(function () { ajax('POST', path, '#content') })
      return
    }
    ajax('POST', path, '#content')
  }
  // 閲覧モードで本文（.markdown-body）だけを選択する（ウィンドウ全体の選択を防ぐ）。成否を返す。
  function selectPreviewAll() {
    var body = document.querySelector('.preview-scroll .markdown-body')
    var sel = window.getSelection && window.getSelection()
    if (!body || !sel) return false
    var range = document.createRange()
    range.selectNodeContents(body)
    sel.removeAllRanges()
    sel.addRange(range)
    return true
  }

  function editExec(cmd) {
    var ta = document.querySelector('.editor-input')
    if (ta) {
      // 編集モード: textarea にフォーカスしてネイティブ編集コマンド。
      ta.focus()
      if (cmd === 'selectAll') {
        ta.select()
        return
      }
      if (cmd === 'paste') {
        pasteFromClipboard(ta)
        return
      }
      try {
        document.execCommand(cmd)
      } catch (e) {
        /* 実行不可の環境は無視。キーボードでは標準動作する。 */
      }
      return
    }
    // 閲覧モード: 意味を持つのは全選択とコピーのみ（取り消し等は編集メニューで無効化済み）。
    if (cmd === 'selectAll') {
      selectPreviewAll()
    } else if (cmd === 'copy') {
      try {
        document.execCommand('copy')
      } catch (e) {
        /* コピー不可環境は無視（Ctrl+C は標準動作する）。 */
      }
    }
  }
  // 「貼り付け」。Chromium 系 WebView は script からのクリップボード読み取りを禁止しており
  // （execCommand('paste') は無効、navigator.clipboard.readText() は権限要求）、WebView 側だけでは
  // 実装できない。そこで OS のクリップボードは Go 側（POST /clipboard/read → Host.ClipboardText）
  // から受け取り、挿入は execCommand('insertText') で行う——この経路ならネイティブの取り消し履歴に
  // 載り、htmx が待つ input イベントも発火する（＝編集がサーバへ送られ dirty になる）。
  // Ctrl+V は WebView が自前で処理するためここは通らない。
  // 通信に fetch ではなく XHR を使うのは、htmx が全 OS で実証済みの経路に揃えるため
  //（アセットは macOS＝WKURLSchemeHandler / Linux＝WebKit の URI スキームハンドラ経由という
  // 特殊な配信で、glue が持ち込む通信手段を増やしたくない）。
  function pasteFromClipboard(ta) {
    var xhr = new XMLHttpRequest()
    xhr.open('POST', '/clipboard/read')
    xhr.onload = function () {
      if (xhr.status < 200 || xhr.status >= 300) return // 取得不可は無視（Ctrl+V は標準動作する）
      var text = xhr.responseText
      if (!text) return
      ta.focus()
      if (document.execCommand('insertText', false, text)) return
      // insertText が使えない環境: 選択範囲を直接置換し input を自前で発火する（取り消しは効かない）。
      ta.setRangeText(text, ta.selectionStart, ta.selectionEnd, 'end')
      ta.dispatchEvent(new Event('input', { bubbles: true }))
    }
    xhr.onerror = function () {
      /* クリップボード取得不可は無視。 */
    }
    xhr.send()
  }

  var R = window.runtime
  if (R && R.EventsOn) {
    R.EventsOn('menu:new', function () { ajax('POST', '/tabs/new', '#content') })
    R.EventsOn('menu:open', function () { ajax('POST', '/tabs/open', '#content') })
    R.EventsOn('menu:save', function () { saveActive('/active/save') })
    R.EventsOn('menu:saveAs', function () { saveActive('/active/save-as') })
    R.EventsOn('menu:print', function () { ajax('POST', '/active/print', '#content') })
    R.EventsOn('menu:toggleMode', function () { ajax('POST', '/active/mode', '#content') })
    R.EventsOn('menu:toggleSidebar', function () { ajax('POST', '/sidebar/toggle', '#sidebar', 'outerHTML') })
    R.EventsOn('menu:about', function () { ajax('POST', '/doc/about', '#content') })
    R.EventsOn('menu:license', function () { ajax('POST', '/doc/license', '#content') })
    R.EventsOn('menu:settings', function () { ajax('POST', '/settings/toggle', '#settings', 'outerHTML') })
    R.EventsOn('menu:style-import', function () { ajax('POST', '/settings/import', '#content') })
    R.EventsOn('menu:style-export', function () { ajax('POST', '/settings/export', '#settings', 'outerHTML') })
    ;['undo', 'redo', 'cut', 'copy', 'paste', 'selectAll'].forEach(function (cmd) {
      R.EventsOn('menu:' + cmd, function () { editExec(cmd) })
    })
    R.EventsOn('menu:find', function () { openFind() })
    // 終了要求（beforeClose が未保存を検知して発火）→ 終了ループ開始（サーバが未保存タブを順に確認）。
    R.EventsOn('app:request-quit', function () {
      if (window.htmx) window.htmx.ajax('POST', '/quit/request', { target: '#dialog-host', swap: 'innerHTML' })
    })
    // 単一インスタンス IPC / ファイル関連付け: 実行中アプリで指定ファイルを開く。
    R.EventsOn('ipc:open-file', function (path) {
      if (path && window.htmx) window.htmx.ajax('POST', '/tabs/open-path', { target: '#content', values: { path: path } })
    })
  }

  // --- 5. 印刷トリガ -----------------------------------------------------
  // /active/print が HX-Trigger-After-Settle: do-print を返す。htmx は settle 後に
  // htmx:afterSettle を発火してから後設定トリガを流すため（afterSettleCallback）、do-print を
  // 受けた時点で mermaid の描画は既にキューへ積まれている。そこで**キューの完了を待ってから**
  // 印刷する（以前は 250ms の固定待ちで、図が多い文書や遅い環境では未描画のまま印刷され得た）。
  // 描画が返らない場合に印刷できなくならないよう、待ち時間には上限を設ける。
  // macOS の WKWebView は window.print() を無視するため Go の Print へ委譲する
  // （macOS は自前のネイティブ印刷パネル、Windows/Linux は従来どおり window.print() が実行される）。
  var PRINT_MAX_WAIT = 5000
  document.body.addEventListener('do-print', function () {
    var giveUp = new Promise(function (resolve) {
      setTimeout(resolve, PRINT_MAX_WAIT)
    })
    Promise.race([mermaidChain, giveUp]).then(function () {
      // 差し込んだ SVG のレイアウトが済んでから印刷する（1 フレーム待つ）。
      requestAnimationFrame(function () {
        setTimeout(function () {
          if (window.go) window.go.main.App.Print()
        }, 50)
      })
    })
  })

  // --- 6. ページ内検索（Ctrl+F / Cmd+F）---------------------------------
  // 可視テキスト層を走査し CSS Custom Highlight API でハイライトする（DOM は書き換えない）。
  // 対象は表示中のモードで切り替える: 閲覧=.markdown-body / 編集=.hl オーバーレイ。
  // バー自体はサーバ断片（POST /find/open）で #find-host に注入し、ここで挙動を配線する。
  var HL_ALL = 'mm-find'
  var HL_CUR = 'mm-find-current'
  var findSupported = typeof CSS !== 'undefined' && CSS.highlights && typeof Highlight !== 'undefined'
  var find = { matches: [], index: 0, container: null, extra: null }

  function findBar() {
    return document.querySelector('.find-bar')
  }

  // 検索対象（可視テキストのルート）とスクロールコンテナ、および編集モードで
  // .hl と同期スクロールさせる textarea（extra）を返す。無ければ null。
  function findTargets() {
    var md = document.querySelector('.preview-scroll .markdown-body')
    if (md) return { root: md, container: document.querySelector('.preview-scroll'), extra: null }
    var hl = document.querySelector('.editor .hl')
    if (hl) return { root: hl, container: hl, extra: document.querySelector('.editor .editor-input') }
    return null
  }

  function clearFindHighlights() {
    if (findSupported) {
      CSS.highlights.delete(HL_ALL)
      CSS.highlights.delete(HL_CUR)
    }
    find.matches = []
    find.index = 0
  }

  function applyFindHighlights() {
    if (!findSupported) return
    if (!find.matches.length) {
      CSS.highlights.delete(HL_ALL)
      CSS.highlights.delete(HL_CUR)
      return
    }
    CSS.highlights.set(HL_ALL, new Highlight(...find.matches))
    var cur = find.matches[find.index]
    if (cur) CSS.highlights.set(HL_CUR, new Highlight(cur))
    else CSS.highlights.delete(HL_CUR)
  }

  // query の一致範囲を集めてハイライト。keepScroll=true なら現在位置を保ちスクロールしない
  // （本文再描画で Range が無効化された際の作り直し用）。
  function computeFind(query, keepScroll) {
    var prev = find.index
    clearFindHighlights()
    var t = findTargets()
    if (!t || !query) {
      updateFindCount()
      return
    }
    find.container = t.container
    find.extra = t.extra
    var needle = query.toLowerCase()
    // mermaid が SVG 内に注入する <style> 等の不可視テキストは対象外にする
    // （一致数が膨れ、現在一致がそこへ当たるとスクロール・ハイライトが狂うため）。
    var walker = document.createTreeWalker(t.root, NodeFilter.SHOW_TEXT, {
      acceptNode: function (n) {
        var tag = n.parentNode && n.parentNode.nodeName ? n.parentNode.nodeName.toLowerCase() : ''
        return tag === 'style' || tag === 'script' ? NodeFilter.FILTER_REJECT : NodeFilter.FILTER_ACCEPT
      }
    })
    var node
    var matches = []
    while ((node = walker.nextNode())) {
      var hay = node.nodeValue.toLowerCase()
      var from = 0
      var idx
      while ((idx = hay.indexOf(needle, from)) !== -1) {
        var r = document.createRange()
        r.setStart(node, idx)
        r.setEnd(node, idx + needle.length)
        matches.push(r)
        from = idx + needle.length
      }
    }
    find.matches = matches
    find.index = keepScroll && prev < matches.length ? prev : 0
    applyFindHighlights()
    if (matches.length && !keepScroll) scrollToCurrent()
    updateFindCount()
  }

  // 現在の一致がコンテナの中央付近に来るよう scrollTop を調整（一致がビュー外でも rect は相対で得られる）。
  function scrollToCurrent() {
    var m = find.matches[find.index]
    if (!m || !find.container) return
    var rr = m.getBoundingClientRect()
    var cr = find.container.getBoundingClientRect()
    var top = find.container.scrollTop + (rr.top - cr.top) - find.container.clientHeight / 2 + rr.height / 2
    if (top < 0) top = 0
    find.container.scrollTop = top
    if (find.extra) find.extra.scrollTop = top // 編集モード: textarea も同じ位置へ（.hl と同一ジオメトリ）
  }

  function gotoFind(delta) {
    var n = find.matches.length
    if (!n) return
    find.index = (find.index + delta + n) % n // 循環
    applyFindHighlights()
    scrollToCurrent()
    updateFindCount()
  }

  function updateFindCount() {
    var bar = findBar()
    if (!bar) return
    var span = bar.querySelector('[data-find-count]')
    var input = bar.querySelector('[data-find-input]')
    if (!span) return
    var q = input ? input.value : ''
    if (!q) {
      span.textContent = ''
      return
    }
    var total = find.matches.length
    span.textContent = (total ? find.index + 1 : 0) + '/' + total
  }

  function openFind() {
    var bar = findBar()
    if (bar) {
      var input = bar.querySelector('[data-find-input]')
      if (input) {
        input.focus()
        input.select()
      }
      return
    }
    // 未表示ならサーバ断片を #find-host に注入。配線とフォーカスは注入後（setupFindBar）で行う。
    if (window.htmx) window.htmx.ajax('POST', '/find/open', { target: '#find-host', swap: 'innerHTML' })
  }

  function closeFind() {
    clearFindHighlights()
    var host = document.getElementById('find-host')
    if (host) host.innerHTML = ''
    var t = findTargets()
    if (t && t.extra) t.extra.focus() // 編集モードは textarea にフォーカスを戻す
  }

  function setupFindBar() {
    var bar = findBar()
    if (!bar || bar.dataset.mmReady) return
    bar.dataset.mmReady = '1'
    var input = bar.querySelector('[data-find-input]')
    if (input) {
      input.addEventListener('input', function () {
        computeFind(input.value, false)
      })
      input.addEventListener('keydown', function (e) {
        if (e.key === 'Enter') {
          e.preventDefault()
          gotoFind(e.shiftKey ? -1 : 1)
        } else if (e.key === 'Escape') {
          e.preventDefault()
          closeFind()
        }
      })
      input.focus()
    }
    var prev = bar.querySelector('[data-find-prev]')
    var next = bar.querySelector('[data-find-next]')
    var close = bar.querySelector('[data-find-close]')
    if (prev) prev.addEventListener('click', function () { gotoFind(-1) })
    if (next) next.addEventListener('click', function () { gotoFind(1) })
    if (close) close.addEventListener('click', closeFind)
  }

  // 本文が再描画されたら（モード切替・スタイル変更など）一致を作り直す（Range が無効化されるため）。
  function refreshFindIfOpen() {
    var bar = findBar()
    if (!bar) return
    var input = bar.querySelector('[data-find-input]')
    if (input && input.value) computeFind(input.value, true)
  }

  // --- 7. 外部リンクの遷移制御 -------------------------------------------
  // WebView 内で <a> をクリックすると WebView 自体が外部 URL へ遷移してアプリが壊れる。
  // capture フェーズで横取りする（旧 links.ts 相当）:
  //   #...              → 見出し id へスクロール（location.hash を書き換えない）
  //   http/https/mailto → 確認ダイアログ（サーバ）→「はい」で OS ブラウザ（Host.OpenURL）
  //   その他・相対       → 遷移をブロック
  function findAnchor(e) {
    var path = (e.composedPath && e.composedPath()) || []
    for (var i = 0; i < path.length; i++) {
      if (path[i] && path[i].tagName === 'A') return path[i]
    }
    var node = e.target
    while (node) {
      if (node.tagName === 'A') return node
      node = node.parentNode
    }
    return null
  }
  // externalHref は <a> が「OS ブラウザへ委譲すべき外部リンク」ならその絶対 URL を、そうでなければ
  // '' を返す。クリック経路（§5.11）と右クリックメニュー（§5.10）が判定を共有する。
  // a.href は解決済みの絶対 URL なので、スキームだけでは判定できない: Wails の配信元は
  // **Windows だけ http://wails.localhost/**（macOS/Linux は wails://）で、文書内アンカーや
  // 相対リンクも http:// に化けて外部リンクと誤判定されるため、raw href と自オリジンで除外する。
  function externalHref(a) {
    if (!a) return ''
    var raw = a.getAttribute('href')
    if (!raw || raw.charAt(0) === '#') return '' // 文書内アンカー
    if (!/^(https?|mailto):/i.test(a.href)) return '' // wails:// 等（非外部スキーム）
    var origin = window.location.origin
    if (origin && a.href.indexOf(origin + '/') === 0) return '' // 相対リンク（自オリジンに解決）
    return a.href
  }
  document.addEventListener(
    'click',
    function (e) {
      if (e.defaultPrevented) return
      var a = findAnchor(e)
      if (!a) return
      var rawHref = a.getAttribute('href')
      if (!rawHref) return
      // 文書内アンカー: 見出しへスクロール。
      if (rawHref.charAt(0) === '#') {
        e.preventDefault()
        var id = rawHref.slice(1)
        try {
          id = decodeURIComponent(id)
        } catch (err) {
          /* 不正なエンコードはそのまま使用 */
        }
        if (id) {
          var target = document.getElementById(id)
          if (target) target.scrollIntoView({ behavior: 'smooth', block: 'start' })
        }
        return
      }
      // 外部リンクは確認ダイアログへ。
      var ext = externalHref(a)
      if (ext) {
        e.preventDefault()
        if (window.htmx) {
          window.htmx.ajax('POST', '/link/confirm', {
            target: '#dialog-host',
            swap: 'innerHTML',
            values: { url: ext },
          })
        }
        return
      }
      // その他スキーム・相対は WebView 遷移を防ぐためブロック。
      e.preventDefault()
    },
    true
  )

  // --- 8. 設定パネルの対入力同期 ----------------------------------------
  // data-pair 内の入力どうし（スライダー↔数値 / カラー↔HEX）で値を鏡写しにする。
  // value のプログラム的設定は input を発火しないため、htmx の二重 POST は起きない
  // （ユーザーが操作した側だけがサーバへ送り #styleblock を更新する）。
  document.addEventListener('input', function (e) {
    var el = e.target
    if (!el || !el.matches || !el.matches('[data-pair] input')) return
    var pair = el.closest('[data-pair]')
    if (!pair) return
    var inputs = pair.querySelectorAll('input')
    for (var i = 0; i < inputs.length; i++) {
      if (inputs[i] !== el && inputs[i].value !== el.value) inputs[i].value = el.value
    }
  })

  // --- 9. 右クリックメニュー ---------------------------------------------
  // 本文（#content）内の contextmenu を横取りし、サーバ断片（POST /ctxmenu/open）を
  // #ctxmenu-host に注入する。項目の並びは常に同一でサーバが活性・非活性だけを決め、
  // 選択の有無とリンク URL はここで収集して渡す。編集操作はネイティブメニューと同じ editExec、
  // 「リンクを開く...」は既存のリンククリックと同じ /link/confirm フローに合流する。
  function ctxMenu() {
    return document.querySelector('#ctxmenu-host .ctxmenu')
  }
  function closeCtxMenu() {
    var host = document.getElementById('ctxmenu-host')
    if (host && host.firstChild) host.innerHTML = ''
  }
  // 取り消し／やり直しの残量を問い合わせる（メニュー項目の活性判定用）。textarea はネイティブの
  // 取り消し履歴を使っており、その残量を知る手段は queryCommandEnabled しかない（自前スタックを
  // 持つと Ctrl+Z の標準動作と二重管理になる）。非推奨 API のため未対応環境では例外や false に
  // なり得るので、その場合は「可能」として扱い項目を活性のままにする——押しても何も起きないだけで、
  // 実際には使える操作を誤って塞がないことを優先する。
  function canEditCmd(cmd) {
    if (!document.queryCommandEnabled) return true
    try {
      return document.queryCommandEnabled(cmd)
    } catch (e) {
      return true
    }
  }
  // 現在のモードで本文に選択があるか（編集＝textarea の選択 / 閲覧＝ウィンドウの選択）。
  function hasContentSelection() {
    var ta = document.querySelector('.editor-input')
    if (ta) return ta.selectionStart !== ta.selectionEnd
    var sel = window.getSelection && window.getSelection()
    return !!sel && !sel.isCollapsed
  }
  // コンテキストクリック直前の選択状態（下の mousedown で採り、contextmenu で使い捨てる）。
  // null は「直前にコンテキストクリックが無い」＝キーボード起動（Windows/Linux の Menu キー・
  // Shift+F10）を意味し、その場合は従来どおり contextmenu 時点の実測にフォールバックする。
  var ctxClickSel = null
  function snapshotSelection() {
    var ta = document.querySelector('.editor-input')
    if (ta) {
      return { ta: ta, start: ta.selectionStart, end: ta.selectionEnd, dir: ta.selectionDirection }
    }
    var sel = window.getSelection && window.getSelection()
    return { ta: null, hadSel: !!sel && !sel.isCollapsed }
  }
  function snapshotHasSelection(snap) {
    return snap.ta ? snap.start !== snap.end : snap.hadSel
  }
  // クリック時点の選択へ戻す。mousedown の既定動作は抑止済みだが、macOS の WebKit は
  // コンテキストクリックでポインタ下の単語を自動選択する（EditingBehavior が Mac のときだけ
  // 有効な挙動。Windows の Blink・Linux の WebKitGTK には無い）ため、抑止で止まらなかった場合の
  // 保険として戻す。閲覧モードは範囲の復元まではせず、無かった選択が生じた場合に解除するだけ。
  function restoreSelection(snap) {
    if (!snap) return
    if (snap.ta) {
      if (snap.ta.selectionStart !== snap.start || snap.ta.selectionEnd !== snap.end) {
        try {
          snap.ta.setSelectionRange(snap.start, snap.end, snap.dir)
        } catch (e) {
          /* 差し替え済み等で設定できない場合は諦める */
        }
      }
      return
    }
    if (snap.hadSel) return
    var sel = window.getSelection && window.getSelection()
    if (sel && !sel.isCollapsed) sel.removeAllRanges()
  }
  document.addEventListener('contextmenu', function (e) {
    var snap = ctxClickSel
    ctxClickSel = null // 使い捨て（古い値をあとのキーボード起動に持ち越さない）
    var content = document.getElementById('content')
    if (!content || !content.contains(e.target)) {
      closeCtxMenu() // 本文の外は既定動作のまま（メニューだけ閉じる）
      return
    }
    e.preventDefault()
    restoreSelection(snap)
    var x = e.clientX
    var y = e.clientY
    var hasSel = snap ? snapshotHasSelection(snap) : hasContentSelection()
    var link = externalHref(findAnchor(e))
    if (!window.htmx) return
    window.htmx
      .ajax('POST', '/ctxmenu/open', {
        target: '#ctxmenu-host',
        swap: 'innerHTML',
        values: {
          sel: hasSel ? '1' : '',
          undo: canEditCmd('undo') ? '1' : '',
          redo: canEditCmd('redo') ? '1' : '',
          link: link,
        },
      })
      .then(function () {
        var m = ctxMenu()
        if (!m) return
        // クリック位置に表示し、画面からはみ出す分は内側へ寄せる（CSS は位置決めまで非表示）。
        var left = Math.max(0, Math.min(x, window.innerWidth - m.offsetWidth - 4))
        var top = Math.max(0, Math.min(y, window.innerHeight - m.offsetHeight - 4))
        m.style.left = left + 'px'
        m.style.top = top + 'px'
        m.style.visibility = 'visible'
      })
  })
  // mousedown の既定動作（フォーカス移動・選択解除・キャレット移動）を 2 か所で抑止する。
  //   1. メニュー内: 本文の選択を保ったまま「コピー」等を実行するため。メニュー外はクローズ。
  //   2. 本文内のコンテキストクリック: 選択とキャレットを一切変えずにメニューを出すため、
  //      選択の有無にかかわらず抑止する。既定に任せると (a) 選択範囲の外を右クリックしたときに
  //      選択が解除され「コピー」等が実質使えなくなり、(b) macOS ではポインタ下の単語が自動選択
  //      されて「選択していないのに切り取り／コピーが使える」状態になる。抑止の結果、右クリックは
  //      キャレットを動かさず、「貼り付け」は直前のキャレット位置に入る（3 OS 共通）。
  //      macOS の Ctrl+クリックは右ボタンではなく button 0＋ctrlKey で来るため、これも同様に扱う
  //      （Windows/Linux の Ctrl+左クリックは本アプリで機能を持たず、click は従来どおり発火する）。
  document.addEventListener(
    'mousedown',
    function (e) {
      var m = ctxMenu()
      if (m) {
        if (m.contains(e.target)) {
          e.preventDefault()
          return
        }
        closeCtxMenu()
      }
      var content = document.getElementById('content')
      var isContextClick = e.button === 2 || (e.button === 0 && e.ctrlKey)
      if (isContextClick && content && content.contains(e.target)) {
        ctxClickSel = snapshotSelection() // この時点の選択が contextmenu の判定材料になる
        e.preventDefault()
      } else {
        ctxClickSel = null
      }
    },
    true
  )
  document.addEventListener('click', function (e) {
    var m = ctxMenu()
    if (!m || !m.contains(e.target)) return
    var btn = e.target.closest ? e.target.closest('.ctxmenu-item') : null
    if (!btn || btn.disabled) return
    var cmd = btn.getAttribute('data-ctx-cmd')
    var openURL = btn.getAttribute('data-ctx-link-open')
    var copyURL = btn.getAttribute('data-ctx-link-copy')
    closeCtxMenu()
    if (cmd) {
      editExec(cmd)
    } else if (openURL && window.htmx) {
      window.htmx.ajax('POST', '/link/confirm', {
        target: '#dialog-host',
        swap: 'innerHTML',
        values: { url: openURL },
      })
    } else if (copyURL) {
      copyText(copyURL)
    }
  })
  // スクロール・ウィンドウ操作でメニューと表示位置がずれるため閉じる（scroll は capture で拾う）。
  document.addEventListener('scroll', function () { closeCtxMenu() }, true)
  window.addEventListener('resize', closeCtxMenu)
  window.addEventListener('blur', closeCtxMenu)

  // テキストをクリップボードへコピーする（リンク URL 用）。Clipboard API が使えない
  // WebView では一時 textarea ＋ execCommand にフォールバックする。
  function copyText(text) {
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(text).catch(function () { fallbackCopy(text) })
      return
    }
    fallbackCopy(text)
  }
  function fallbackCopy(text) {
    var ta = document.createElement('textarea')
    ta.value = text
    ta.style.position = 'fixed'
    ta.style.opacity = '0'
    document.body.appendChild(ta)
    ta.select()
    try {
      document.execCommand('copy')
    } catch (e) {
      /* コピー不可環境は無視 */
    }
    document.body.removeChild(ta)
  }

})()
