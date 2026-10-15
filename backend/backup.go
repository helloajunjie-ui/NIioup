package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func mimeFormatAttachment(name string) string {
	return mime.FormatMediaType("attachment", map[string]string{"filename": name})
}

// zipWriter 流式写 zip：先 header 后 body，避免整包驻留内存
type zipWriter struct {
	zw *zip.Writer
}

func newZipWriter(w io.Writer) *zipWriter {
	return &zipWriter{zw: zip.NewWriter(w)}
}

// AddFile 把本地文件加入 zip 的指定 entry 名
func (z *zipWriter) AddFile(entry, src string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	h, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	h.Name = entry
	h.Method = zip.Deflate
	w, err := z.zw.CreateHeader(h)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, f)
	return err
}

func (z *zipWriter) Close() error {
	return z.zw.Close()
}

// handleBackup POST /api/admin/backup  一键备份：打包全部文件 + 配置 + 元数据
// 备份包结构：
//
//	config.json
//	data/meta.json
//	files/<stored_name>   （存储名，便于一键恢复回写）
func handleBackup(w http.ResponseWriter, r *http.Request) {
	ts := time.Now().Format("20060102_150405")
	name := "backup_" + ts + ".zip"
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", mimeFormatAttachment(name))

	zw := newZipWriter(w)
	_ = zw.AddFile("config.json", configFile)
	_ = zw.AddFile("data/meta.json", metaFile)

	files := store.All()
	for _, f := range files {
		src := filePathOf(f)
		if _, err := os.Stat(src); err == nil {
			_ = zw.AddFile("files/"+f.StoredName, src)
		}
	}
	zw.Close()
}

// handleRestore POST /api/admin/restore  一键恢复：上传备份 zip，回写文件 + 配置 + 元数据
func handleRestore(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(1 << 30); err != nil { // 兼容大备份包
		writeErr(w, http.StatusBadRequest, "备份包过大或格式错误")
		return
	}
	defer r.MultipartForm.RemoveAll()
	file, _, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "缺少文件字段 file")
		return
	}
	defer file.Close()

	tmp, err := os.MkdirTemp("", "restore_")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "创建临时目录失败")
		return
	}
	defer os.RemoveAll(tmp)

	if err := unzipTo(file, tmp); err != nil {
		writeErr(w, http.StatusBadRequest, "备份包解析失败: "+err.Error())
		return
	}

	// 1) 校验并读取配置与元数据（解析失败即中止，不动现有数据）
	backCfg, err := loadConfigFrom(filepath.Join(tmp, "config.json"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "备份包缺少有效 config.json: "+err.Error())
		return
	}
	metas, err := loadMetaFrom(filepath.Join(tmp, "data", "meta.json"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "备份包缺少有效元数据: "+err.Error())
		return
	}
	// 校验存储名安全
	seen := map[string]bool{}
	for i := range metas {
		sn := metas[i].StoredName
		if sn == "" || sn == "." || sn == ".." || strings.ContainsAny(sn, `/\`) || seen[sn] {
			writeErr(w, http.StatusBadRequest, "备份包元数据存储名非法")
			return
		}
		seen[sn] = true
	}

	dir := absPath(backCfg.StorageDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeErr(w, http.StatusInternalServerError, "存储目录不可用: "+err.Error())
		return
	}

	// 2) 覆盖式回写备份文件到存储目录
	srcFilesDir := filepath.Join(tmp, "files")
	entries, err := os.ReadDir(srcFilesDir)
	if err != nil && !os.IsNotExist(err) {
		writeErr(w, http.StatusInternalServerError, "读取备份文件失败: "+err.Error())
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		in, err := os.Open(filepath.Join(srcFilesDir, e.Name()))
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "读取备份文件失败: "+err.Error())
			return
		}
		cpErr := copyFile(filepath.Join(dir, e.Name()), in, -1)
		in.Close()
		if cpErr != nil {
			writeErr(w, http.StatusInternalServerError, "恢复文件失败: "+cpErr.Error())
			return
		}
	}

	// 3) 应用配置到运行态并保存（端口/存储目录变更需重启生效）
	cfgMu.Lock()
	cfg = backCfg
	cfg.AllowedExtensions = normalizeExts(cfg.AllowedExtensions)
	cfgMu.Unlock()
	if err := SaveConfig(); err != nil {
		writeErr(w, http.StatusInternalServerError, "配置保存失败: "+err.Error())
		return
	}

	// 4) 应用元数据
	store.mu.Lock()
	store.files = metas
	_ = store.save()
	store.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"restored": len(metas),
		"note":     "已恢复；若备份中的端口/存储目录与当前不同，端口需重启生效",
	})
}

// unzipTo 安全解压 zip 到目标目录（过滤路径穿越）
func unzipTo(src io.Reader, dst string) error {
	data, err := io.ReadAll(src)
	if err != nil {
		return err
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	base := filepath.Clean(dst) + string(filepath.Separator)
	for _, zf := range zr.File {
		name := filepath.Clean(filepath.FromSlash(zf.Name))
		if name == "." || name == ".." || filepath.IsAbs(name) || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
			continue
		}
		target := filepath.Join(dst, name)
		if !strings.HasPrefix(target, base) {
			continue
		}
		if zf.FileInfo().IsDir() {
			os.MkdirAll(target, 0o755)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := zf.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(target)
		if err != nil {
			rc.Close()
			return err
		}
		if _, err := io.Copy(out, rc); err != nil {
			rc.Close()
			out.Close()
			return err
		}
		rc.Close()
		out.Close()
	}
	return nil
}

func loadConfigFrom(path string) (Config, error) {
	var c Config
	data, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	// 以默认值为基底合并，保证新增字段有默认值
	base, _ := json.Marshal(defaultConfig())
	m := map[string]json.RawMessage{}
	_ = json.Unmarshal(base, &m)
	fm := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &fm); err != nil {
		return c, err
	}
	for k, v := range fm {
		m[k] = v
	}
	merged, _ := json.Marshal(m)
	err = json.Unmarshal(merged, &c)
	return c, err
}

func loadMetaFrom(path string) ([]FileMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var metas []FileMeta
	err = json.Unmarshal(data, &metas)
	return metas, err
}
