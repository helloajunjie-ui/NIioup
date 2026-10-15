package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const configFile = "config.json"

// Config 是站长可配置的全部运行参数，持久化在 config.json。
type Config struct {
	SiteTitle         string   `json:"site_title"`          // 站点名称
	Port              int      `json:"port"`                // 监听端口
	StorageDir        string   `json:"storage_dir"`         // 文件存放目录（相对或绝对路径）
	FrontendDir       string   `json:"frontend_dir"`        // Vue 构建产物目录
	MaxFileSizeMB     int64    `json:"max_file_size_mb"`    // 单文件大小上限（MB）
	AllowedExtensions []string `json:"allowed_extensions"`  // 允许的后缀（小写、不含点），空=全部允许
	MaxFilesTotal     int      `json:"max_files_total"`     // 文件总数上限，0=不限
	UploadEnabled     bool     `json:"upload_enabled"`      // 是否开放匿名上传
	AutoDeleteDays    int      `json:"auto_delete_days"`    // 超过 N 天自动删除，0=永久保留
	PublicList        bool     `json:"public_list"`         // 首页是否展示最近分享列表
	UploadRatePerMin  int    `json:"upload_rate_per_min"`   // 上传频率：每 IP 每分钟次数，0=不限
	DownloadRatePerMin int   `json:"download_rate_per_min"` // 下载频率：每 IP 每分钟次数，0=不限
	AdCode            string `json:"ad_code"`               // 下载门槛页顶部广告位（HTML），留空则显示倒计时
	WaitSecMin        int    `json:"wait_sec_min"`          // 下载等待最短秒数
	WaitSecMax        int    `json:"wait_sec_max"`          // 下载等待最长秒数（范围内随机）
	AdminPassword     string `json:"admin_password"`        // 管理员密码
}

var (
	cfg   Config
	cfgMu sync.RWMutex
)

func defaultConfig() Config {
	return Config{
		SiteTitle:          "匿名文件仓库",
		Port:               8080,
		StorageDir:         "./data/files",
		FrontendDir:        "../frontend/dist",
		MaxFileSizeMB:      100,
		AllowedExtensions:  []string{},
		MaxFilesTotal:      1000,
		UploadEnabled:      true,
		AutoDeleteDays:     0,
		PublicList:         true,
		UploadRatePerMin:   20,
		DownloadRatePerMin: 60,
		AdCode:             "",
		WaitSecMin:         5,
		WaitSecMax:         20,
		AdminPassword:      "admin123",
	}
}

func LoadConfig() error {
	// 以默认值为基底，用 config.json 覆盖，保证新增字段有默认值
	base, err := json.Marshal(defaultConfig())
	if err != nil {
		return err
	}
	m := map[string]json.RawMessage{}
	if err := json.Unmarshal(base, &m); err != nil {
		return err
	}
	missing := false
	if data, err := os.ReadFile(configFile); err == nil {
		fm := map[string]json.RawMessage{}
		if err := json.Unmarshal(data, &fm); err != nil {
			return err
		}
		for k, v := range fm {
			m[k] = v
		}
	} else if os.IsNotExist(err) {
		missing = true // 首次运行，稍后写入默认配置
	} else {
		return err
	}
	merged, _ := json.Marshal(m)
	if err := json.Unmarshal(merged, &cfg); err != nil {
		return err
	}
	cfg.AllowedExtensions = normalizeExts(cfg.AllowedExtensions)
	if missing {
		return SaveConfig() // 生成默认 config.json
	}
	return nil
}

func SaveConfig() error {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	cfg.AllowedExtensions = normalizeExts(cfg.AllowedExtensions)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(configFile, data)
}

func (c *Config) Address() string {
	port := c.Port
	if port <= 0 {
		port = 8080
	}
	return fmt.Sprintf("0.0.0.0:%d", port)
}

// normalizeExts 去掉前导点、转小写、去掉空项
func normalizeExts(exts []string) []string {
	out := []string{}
	for _, e := range exts {
		e = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(e), "."))
		if e != "" {
			out = append(out, e)
		}
	}
	return out
}

func extAllowed(ext string) bool {
	if len(cfg.AllowedExtensions) == 0 {
		return true
	}
	ext = strings.ToLower(ext)
	for _, e := range cfg.AllowedExtensions {
		if e == ext {
			return true
		}
	}
	return false
}

// absPath 把相对路径解析为相对当前工作目录的绝对路径
func absPath(p string) string {
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}

func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

var errInvalidConfig = errors.New("invalid config")
