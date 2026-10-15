package main

import (
	"crypto/subtle"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ---- 管理员会话（进程内，重启后需重新登录）----

var (
	sessionMu sync.Mutex
	sessions  = map[string]time.Time{} // token -> 过期时间
)

const sessionTTL = 24 * time.Hour

func newSession() string {
	tok := randomID(48)
	sessionMu.Lock()
	sessions[tok] = time.Now().Add(sessionTTL)
	sessionMu.Unlock()
	return tok
}

func validSession(tok string) bool {
	if tok == "" {
		return false
	}
	sessionMu.Lock()
	defer sessionMu.Unlock()
	exp, ok := sessions[tok]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(sessions, tok)
		return false
	}
	return true
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	return r.Header.Get("X-Admin-Token")
}

// requireAdmin 管理员鉴权中间件
func requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !validSession(bearerToken(r)) {
			writeErr(w, http.StatusUnauthorized, "未登录或会话已过期")
			return
		}
		next(w, r)
	}
}

// handleLogin POST /api/admin/login  {password}
func handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(r.Body, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if subtle.ConstantTimeCompare([]byte(req.Password), []byte(cfg.AdminPassword)) != 1 {
		writeErr(w, http.StatusUnauthorized, "密码错误")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "token": newSession()})
}

// handleAdminFiles GET /api/admin/files?q=&offset=&limit=  全量文件列表（含删除口令）
func handleAdminFiles(w http.ResponseWriter, r *http.Request) {
	q := strings.ToLower(r.URL.Query().Get("q"))
	offset, _ := parseIntDefault(r.URL.Query().Get("offset"), 0)
	limit, _ := parseIntDefault(r.URL.Query().Get("limit"), 100)
	if limit < 0 {
		limit = 100
	}

	all := store.All()
	var filtered []FileMeta
	for _, f := range all {
		if q != "" && !(strings.Contains(strings.ToLower(f.Name), q) || strings.Contains(f.ID, q)) {
			continue
		}
		filtered = append(filtered, f)
	}
	total := len(filtered)
	if offset > len(filtered) {
		offset = len(filtered)
	}
	if offset+limit > len(filtered) {
		limit = len(filtered) - offset
	}
	if limit < 0 {
		limit = 0
	}
	page := filtered[offset : offset+limit]

	out := make([]map[string]any, 0, len(page))
	for _, f := range page {
		m := publicMeta(f)
		m["stored_name"] = f.StoredName
		m["delete_token"] = f.DeleteToken
		m["content_type"] = f.ContentType
		out = append(out, m)
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": out, "total": total, "offset": offset})
}

func parseIntDefault(s string, def int) (int, error) {
	if s == "" {
		return def, nil
	}
	return strconv.Atoi(s)
}

// handleAdminDelete DELETE /api/admin/files/{id}
func handleAdminDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	f, ok := store.Delete(id)
	if !ok {
		writeErr(w, http.StatusNotFound, "文件不存在")
		return
	}
	os.Remove(filePathOf(f))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
}

// handleSelfDelete POST /api/delete {token}  上传者凭口令匿名删除（公开接口）
func handleSelfDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if err := decodeJSON(r.Body, &req); err != nil || req.Token == "" {
		writeErr(w, http.StatusBadRequest, "缺少删除口令")
		return
	}
	f, ok := store.GetByToken(req.Token)
	if !ok {
		writeErr(w, http.StatusNotFound, "口令无效或文件已删除")
		return
	}
	store.Delete(f.ID)
	os.Remove(filePathOf(f))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleAdminGetConfig GET /api/admin/config  返回配置（密码打码，仅返回是否已设置）
func handleAdminGetConfig(w http.ResponseWriter, r *http.Request) {
	cfgMu.RLock()
	c := cfg
	cfgMu.RUnlock()
	resp := map[string]any{
		"site_title":         c.SiteTitle,
		"port":               c.Port,
		"storage_dir":        c.StorageDir,
		"frontend_dir":       c.FrontendDir,
		"max_file_size_mb":   c.MaxFileSizeMB,
		"allowed_extensions": c.AllowedExtensions,
		"max_files_total":    c.MaxFilesTotal,
		"upload_enabled":     c.UploadEnabled,
		"auto_delete_days":   c.AutoDeleteDays,
		"public_list":        c.PublicList,
		"has_password":       c.AdminPassword != "",
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleAdminPutConfig PUT /api/admin/config  更新配置（密码留空则不修改）
func handleAdminPutConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SiteTitle         *string  `json:"site_title"`
		Port              *int     `json:"port"`
		StorageDir        *string  `json:"storage_dir"`
		FrontendDir       *string  `json:"frontend_dir"`
		MaxFileSizeMB     *int64   `json:"max_file_size_mb"`
		AllowedExtensions []string `json:"allowed_extensions"`
		MaxFilesTotal     *int     `json:"max_files_total"`
		UploadEnabled     *bool    `json:"upload_enabled"`
		AutoDeleteDays    *int     `json:"auto_delete_days"`
		PublicList        *bool    `json:"public_list"`
		AdminPassword     *string  `json:"admin_password"`
	}
	if err := decodeJSON(r.Body, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}

	cfgMu.Lock()
	if req.SiteTitle != nil && *req.SiteTitle != "" {
		cfg.SiteTitle = *req.SiteTitle
	}
	if req.Port != nil && *req.Port >= 1 && *req.Port <= 65535 {
		cfg.Port = *req.Port
	}
	if req.StorageDir != nil && *req.StorageDir != "" {
		cfg.StorageDir = *req.StorageDir
		if err := os.MkdirAll(absPath(cfg.StorageDir), 0o755); err != nil {
			cfgMu.Unlock()
			writeErr(w, http.StatusBadRequest, "存储目录不可用: "+err.Error())
			return
		}
	}
	if req.FrontendDir != nil && *req.FrontendDir != "" {
		cfg.FrontendDir = *req.FrontendDir
	}
	if req.MaxFileSizeMB != nil && *req.MaxFileSizeMB >= 1 {
		cfg.MaxFileSizeMB = *req.MaxFileSizeMB
	}
	if req.MaxFilesTotal != nil && *req.MaxFilesTotal >= 0 {
		cfg.MaxFilesTotal = *req.MaxFilesTotal
	}
	if req.UploadEnabled != nil {
		cfg.UploadEnabled = *req.UploadEnabled
	}
	if req.AutoDeleteDays != nil && *req.AutoDeleteDays >= 0 {
		cfg.AutoDeleteDays = *req.AutoDeleteDays
	}
	if req.PublicList != nil {
		cfg.PublicList = *req.PublicList
	}
	if req.AdminPassword != nil && *req.AdminPassword != "" {
		cfg.AdminPassword = *req.AdminPassword
	}
	cfg.AllowedExtensions = normalizeExts(req.AllowedExtensions)
	cfgMu.Unlock()

	if err := SaveConfig(); err != nil {
		writeErr(w, http.StatusInternalServerError, "配置保存失败: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
