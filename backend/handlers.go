package main

import (
	"fmt"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
		"site_title":        cfg.SiteTitle,
		"upload_enabled":    cfg.UploadEnabled,
		"allowed_extensions": cfg.AllowedExtensions,
		"max_file_size_mb":  cfg.MaxFileSizeMB,
		"max_files_total":   cfg.MaxFilesTotal,
		"public_list":       cfg.PublicList,
	})
}

// handleUpload POST /api/upload  匿名上传
func handleUpload(w http.ResponseWriter, r *http.Request) {
	if !cfg.UploadEnabled {
		writeErr(w, http.StatusForbidden, "上传已关闭")
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

	// 下载计数
	go store.incDownloads(id)

	// ?dl=1 强制附件下载；默认内联预览
	disposition := "inline"
	if r.URL.Query().Get("dl") == "1" {
		disposition = "attachment"
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": f.Name}))
	w.Header().Set("Content-Type", f.ContentType)
	http.ServeFile(w, r, path)
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

// handlePublicList GET /api/files?limit=20  最近分享列表
func handlePublicList(w http.ResponseWriter, r *http.Request) {
	if !cfg.PublicList {
		writeJSON(w, http.StatusOK, map[string]any{"files": []any{}, "count": 0})
		return
	}
	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	all := store.All()
	if len(all) > limit {
		all = all[:limit]
	}
	out := make([]map[string]any, 0, len(all))
	for _, f := range all {
		out = append(out, publicMeta(f))
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": out, "count": len(out)})
}
