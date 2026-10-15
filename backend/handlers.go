package main

import (
	"fmt"
	"log"
	"math/rand/v2"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// publicMeta 返回给前端的对外可见字段（不含删除口令）
func publicMeta(f FileMeta) map[string]any {
	return map[string]any{
		"id":          f.ID,
		"name":        f.Name,
		"size":        f.Size,
		"ext":         f.Ext,
		"uploaded_at": f.UploadedAt,
		"downloads":   f.Downloads,
		"guest_uid":   f.GuestUID,
		"category":    f.Category,
		"url":         "/f/" + f.ID,
	}
}

// handleInfo GET /api/info 站点公开信息（前端渲染用）
func handleInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"site_title":         cfg.SiteTitle,
		"upload_enabled":     cfg.UploadEnabled,
		"allowed_extensions": cfg.AllowedExtensions,
		"max_file_size_mb":   cfg.MaxFileSizeMB,
		"max_files_total":    cfg.MaxFilesTotal,
		"public_list":        cfg.PublicList,
		"download_mode":      cfg.DownloadMode,
		"ad_code":            cfg.AdCode,
		"wait_sec_min":       cfg.WaitSecMin,
		"wait_sec_max":       cfg.WaitSecMax,
	})
}

// handleUpload POST /api/upload  匿名上传
func handleUpload(w http.ResponseWriter, r *http.Request) {
	if !cfg.UploadEnabled {
		writeErr(w, http.StatusForbidden, "上传已关闭")
		return
	}
	if !uploadLimiter.allow(clientIP(r), cfg.UploadRatePerMin) {
		writeErr(w, http.StatusTooManyRequests, "上传太频繁，请稍后再试")
		return
	}

	// 限制请求体大小：限制值 + 1MB 表单开销
	maxBytes := (cfg.MaxFileSizeMB + 1) << 20
	if cfg.MaxFileSizeMB <= 0 {
		maxBytes = 0 // 不限制
	}
	if maxBytes > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, "请求体过大或格式错误，请检查文件大小限制")
		return
	}
	defer r.MultipartForm.RemoveAll()

	file, header, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "缺少文件字段 file")
		return
	}
	defer file.Close()

	name := filepath.Base(header.Filename)
	if name == "" || name == "." || name == string(filepath.Separator) {
		writeErr(w, http.StatusBadRequest, "无效文件名")
		return
	}
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	if !extAllowed(ext) {
		writeErr(w, http.StatusBadRequest, "不允许的文件类型: ."+ext)
		return
	}
	if cfg.MaxFileSizeMB > 0 && header.Size > cfg.MaxFileSizeMB<<20 {
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("文件超过大小限制 %d MB", cfg.MaxFileSizeMB))
		return
	}
	if cfg.MaxFilesTotal > 0 && store.Count() >= cfg.MaxFilesTotal {
		writeErr(w, http.StatusTooManyRequests, "文件数量已达上限")
		return
	}

	// 游客 UID：客户端可带（持久化），缺失则自动生成
	guestUID := sanitizeField(r.FormValue("guest_uid"), 32)
	if guestUID == "" {
		guestUID = "G" + randomID(10)
	}
	category := sanitizeField(r.FormValue("category"), 30)
	if category == "" {
		category = "未分类"
	}

	// 生成唯一存储名
	var stored string
	for {
		stored = randomStored()
		if !store.hasStoredName(stored) {
			break
		}
	}

	// 若扩展名已知则取对应 MIME，否则探测
	ct := mime.TypeByExtension(filepath.Ext(name))
	if ct == "" {
		ct = "application/octet-stream"
	}

	dst := filepath.Join(absPath(cfg.StorageDir), stored)
	if err := copyFile(dst, file, header.Size); err != nil {
		log.Printf("save file %s: %v", name, err)
		writeErr(w, http.StatusInternalServerError, "文件保存失败")
		return
	}

	meta := FileMeta{
		ID:          randomID(8),
		Name:        name,
		StoredName:  stored,
		Size:        header.Size,
		Ext:         ext,
		ContentType: ct,
		UploadedAt:  time.Now().Unix(),
		DeleteToken: randomID(24),
		GuestUID:    guestUID,
		Category:    category,
	}
	if err := store.Add(meta); err != nil {
		os.Remove(dst)
		writeErr(w, http.StatusInternalServerError, "元数据写入失败")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":           true,
		"id":           meta.ID,
		"name":         meta.Name,
		"size":         meta.Size,
		"ext":          meta.Ext,
		"category":     meta.Category,
		"guest_uid":    meta.GuestUID,
		"url":          "/f/" + meta.ID,
		"delete_token": meta.DeleteToken,
	})
}

// handleDownload GET /f/{id} 或 /f/{id}/{filename}  外链下载（支持 Range 与内联预览）
func handleDownload(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	f, ok := store.Get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	path := filePathOf(f)
	if _, err := os.Stat(path); err != nil {
		writeErr(w, http.StatusNotFound, "文件不存在或已被清理")
		return
	}

	// ?dl=1 强制附件下载：
	//   direct 模式（无广直连）→ 不校验令牌，仅限流；
	//   ad / countdown 模式 → 必须携带门槛页签发的一次性令牌，防止绕过门槛
	dl := r.URL.Query().Get("dl") == "1"
	if dl {
		if cfg.DownloadMode == "direct" {
			if !downloadLimiter.allow(clientIP(r), cfg.DownloadRatePerMin) {
				writeErr(w, http.StatusTooManyRequests, "下载太频繁，请稍后再试")
				return
			}
		} else if !consumeDownloadToken(r.URL.Query().Get("t"), id) {
			writeErr(w, http.StatusForbidden, "下载链接已失效，请从下载页重新获取")
			return
		}
	} else {
		// 内联预览：仅限流，不校验令牌
		if !downloadLimiter.allow(clientIP(r), cfg.DownloadRatePerMin) {
			writeErr(w, http.StatusTooManyRequests, "下载太频繁，请稍后再试")
			return
		}
	}

	// 下载计数
	go store.incDownloads(id)

	disposition := "inline"
	if dl {
		disposition = "attachment"
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": f.Name}))
	w.Header().Set("Content-Type", f.ContentType)
	http.ServeFile(w, r, path)
}

// ---- 一次性下载令牌（下载门槛页签发，单次有效，防直链绕过） ----

type dlToken struct {
	id  string
	exp time.Time
}

var dlTokens sync.Map // token -> dlToken

func issueDownloadToken(id string) string {
	tok := randomID(48)
	dlTokens.Store(tok, dlToken{id: id, exp: time.Now().Add(3 * time.Minute)})
	return tok
}

func consumeDownloadToken(tok, id string) bool {
	if tok == "" {
		return false
	}
	v, ok := dlTokens.LoadAndDelete(tok)
	if !ok {
		return false
	}
	t := v.(dlToken)
	return t.id == id && time.Now().Before(t.exp)
}

// handleGateStart POST /api/dl/start {id}  下载门槛页倒计时结束后调用：签发一次性下载令牌
func handleGateStart(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	if err := decodeJSON(r.Body, &req); err != nil || req.ID == "" {
		writeErr(w, http.StatusBadRequest, "缺少文件 ID")
		return
	}
	if !downloadLimiter.allow(clientIP(r), cfg.DownloadRatePerMin) {
		writeErr(w, http.StatusTooManyRequests, "下载太频繁，请稍后再试")
		return
	}
	f, ok := store.Get(req.ID)
	if !ok {
		writeErr(w, http.StatusNotFound, "文件不存在")
		return
	}
	tok := issueDownloadToken(req.ID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "token": tok, "url": "/f/" + f.ID + "?dl=1&t=" + tok})
}

// handleInfoFile GET /f/{id}/info  单文件公开信息
func handleInfoFile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	f, ok := store.Get(id)
	if !ok {
		writeErr(w, http.StatusNotFound, "文件不存在")
		return
	}
	writeJSON(w, http.StatusOK, publicMeta(f))
}

// handlePublicList GET /api/files?limit=&q=&category=&random=1  分享列表
// 默认（无搜索、无分类）随机展示 limit 条，避免数据过多时全量加载浪费资源；
// 搜索/分类仍精确过滤；total 与 categories 始终按全量统计。
func handlePublicList(w http.ResponseWriter, r *http.Request) {
	if !cfg.PublicList {
		writeJSON(w, http.StatusOK, map[string]any{"files": []any{}, "count": 0, "total": 0, "categories": []any{}})
		return
	}
	q := strings.ToLower(r.URL.Query().Get("q"))
	cat := r.URL.Query().Get("category")
	random := r.URL.Query().Get("random") == "1"
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}

	all := store.All()

	// 汇总可用分类（按文件数降序）
	catCount := map[string]int{}
	for _, f := range all {
		catCount[f.Category]++
	}
	type kv struct{ k string; n int }
	var cats []kv
	for k, n := range catCount {
		cats = append(cats, kv{k, n})
	}
	sort.Slice(cats, func(i, j int) bool { return cats[i].n > cats[j].n })
	categories := make([]string, 0, len(cats))
	for _, c := range cats {
		categories = append(categories, c.k)
	}

	// 先过滤
	var matched []FileMeta
	for _, f := range all {
		if q != "" && !strings.Contains(strings.ToLower(f.Name), q) {
			continue
		}
		if cat != "" && cat != "所有" && f.Category != cat {
			continue
		}
		matched = append(matched, f)
	}

	// 默认视图：随机抽样
	if random && q == "" && (cat == "" || cat == "所有") {
		rand.Shuffle(len(matched), func(i, j int) { matched[i], matched[j] = matched[j], matched[i] })
	}
	if limit > 0 && len(matched) > limit {
		matched = matched[:limit]
	}

	out := make([]map[string]any, 0, len(matched))
	for _, f := range matched {
		out = append(out, publicMeta(f))
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": out, "count": len(out), "total": len(all), "categories": categories})
}
