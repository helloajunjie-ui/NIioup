package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// decodeJSON 解析请求体 JSON，容忍开头 UTF-8 BOM（部分客户端/工具会带）
func decodeJSON(r io.Reader, v any) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	return json.Unmarshal(data, v)
}

const idAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// randomID 生成短 ID（默认 8 位）
func randomID(n int) string {
	if n <= 0 {
		n = 8
	}
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// 极低概率回退到时间戳
		return strings.ToLower(hex.EncodeToString([]byte(time.Now().String())))
	}
	for i := range b {
		b[i] = idAlphabet[int(b[i])%len(idAlphabet)]
	}
	return string(b)
}

// randomStored 生成存储文件名（32 位 hex）
func randomStored() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return randomID(32)
	}
	return hex.EncodeToString(b)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write json: %v", err)
	}
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg})
}

func filePathOf(f FileMeta) string {
	return filepath.Join(absPath(cfg.StorageDir), f.StoredName)
}

// detectContentType 用扩展名推断 MIME，再回退到探测
func detectContentType(name string) string {
	if ct := mime.TypeByExtension(filepath.Ext(name)); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

// copyFile 落盘并保证不残留半成品
func copyFile(dst string, src io.Reader, size int64) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp := dst + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	_, cpErr := io.Copy(out, src)
	closeErr := out.Close()
	if cpErr != nil {
		os.Remove(tmp)
		return cpErr
	}
	if closeErr != nil {
		os.Remove(tmp)
		return closeErr
	}
	if size >= 0 {
		if fi, err := os.Stat(tmp); err == nil && fi.Size() != size {
			os.Remove(tmp)
			return errSizeMismatch
		}
	}
	return os.Rename(tmp, dst)
}

var errSizeMismatch = &textError{"文件大小与声明不一致"}

type textError struct{ s string }

func (e *textError) Error() string { return e.s }

// sanitizeField 清理表单字段：去掉控制字符与路径分隔符，去首尾空白，限制长度
func sanitizeField(s string, maxLen int) string {
	s = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 || r == '/' || r == '\\' {
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if maxLen > 0 {
		rs := []rune(s)
		if len(rs) > maxLen {
			s = string(rs[:maxLen])
		}
	}
	return s
}
