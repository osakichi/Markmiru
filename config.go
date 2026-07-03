package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// 設定・セッションの永続化。Go 標準パッケージ（encoding/json, os, path/filepath）のみで実装。
// 設計: docs/アーキテクチャ・画面設計.md §5.6, §7

// SessionFile は復元対象ファイル1件（パス）。
type SessionFile struct {
	Path string `json:"path"`
}

// Session は前回のセッション（開いていたファイル群とアクティブ位置）。
type Session struct {
	Files       []SessionFile `json:"files"`
	ActiveIndex int           `json:"activeIndex"`
}

// Config は永続化する設定全体。
// ユーザースタイルは JSON 文字列（style.Style 配列）として保持する（web.State が入出力する）。
type Config struct {
	Session       Session `json:"session"`
	SidebarOpen   bool    `json:"sidebarOpen"`
	StylesJson    string  `json:"stylesJson"`
	ActiveStyleId string  `json:"activeStyleId"`

	// ウィンドウ状態。更新は Go 側（beforeClose の saveWindowState）のみで、
	// セッション保存（persistSession）には含めない（既存値を保持する）。
	WindowWidth     int  `json:"windowWidth"`
	WindowHeight    int  `json:"windowHeight"`
	WindowMaximised bool `json:"windowMaximised"`
}

// defaultWindowWidth / defaultWindowHeight は初回起動時のウィンドウサイズ。
const (
	defaultWindowWidth  = 1024
	defaultWindowHeight = 768
)

func defaultConfig() Config {
	return Config{
		Session:         Session{Files: []SessionFile{}, ActiveIndex: -1},
		SidebarOpen:     true,
		StylesJson:      "",
		ActiveStyleId:   "light",
		WindowWidth:     defaultWindowWidth,
		WindowHeight:    defaultWindowHeight,
		WindowMaximised: false,
	}
}

func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "Markmiru", "config.json"), nil
}

// LoadConfig は設定を読み込む。未作成（初回起動）や破損時は既定値を返す。
func (a *App) LoadConfig() (Config, error) {
	path, err := configPath()
	if err != nil {
		return defaultConfig(), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return defaultConfig(), nil
	}
	cfg := defaultConfig()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return defaultConfig(), nil
	}
	return cfg, nil
}

// writeConfig は Config をそのまま config.json へ書き込む（マージなし）。
func writeConfig(cfg Config) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
