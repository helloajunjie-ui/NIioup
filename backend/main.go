package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	if err := LoadConfig(); err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	if err := InitStore(); err != nil {
		log.Fatalf("初始化存储失败: %v", err)
	}
	if n := PurgeExpired(); n > 0 {
		log.Printf("清理过期文件 %d 个", n)
	}

	mux := http.NewServeMux()
	registerRoutes(mux)

	addr := cfg.Address()
	log.Printf("%s 已启动: http://%s", cfg.SiteTitle, addr)
	log.Printf("存储目录: %s", absPath(cfg.StorageDir))
	log.Printf("前端目录: %s", absPath(cfg.FrontendDir))

	srv := &http.Server{
		Addr:              addr,
		Handler:           logRequests(mux),
		ReadHeaderTimeout: 15 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func registerRoutes(mux *http.ServeMux) {
	// 公开
	mux.HandleFunc("GET /api/info", handleInfo)
	mux.HandleFunc("POST /api/upload", handleUpload)
	mux.HandleFunc("GET /api/files", handlePublicList)
	mux.HandleFunc("POST /api/delete", handleSelfDelete)
	mux.HandleFunc("POST /api/dl/start", handleGateStart)

	mux.HandleFunc("GET /f/{id}", handleDownload)
	mux.HandleFunc("GET /f/{id}/{filename...}", handleDownload)
	mux.HandleFunc("GET /f/{id}/info", handleInfoFile)

	// 管理员
	mux.HandleFunc("POST /api/admin/login", handleLogin)
	mux.HandleFunc("GET /api/admin/files", requireAdmin(handleAdminFiles))
	mux.HandleFunc("DELETE /api/admin/files/{id}", requireAdmin(handleAdminDelete))
	mux.HandleFunc("GET /api/admin/config", requireAdmin(handleAdminGetConfig))
	mux.HandleFunc("PUT /api/admin/config", requireAdmin(handleAdminPutConfig))
	mux.HandleFunc("POST /api/admin/backup", requireAdmin(handleBackup))
	mux.HandleFunc("POST /api/admin/restore", requireAdmin(handleRestore))

	// 前端静态资源（兜底）
	mux.Handle("/", staticHandler())
}

// staticHandler 托管 Vue 构建产物，SPA 路由回退到 index.html
func staticHandler() http.Handler {
	dist := absPath(cfg.FrontendDir)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := os.Stat(filepath.Join(dist, "index.html")); err != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "<html><body style='font-family:system-ui;padding:40px'><h2>前端未构建</h2><p>请进入 frontend 目录执行 <code>npm install &amp;&amp; npm run build</code> 后再刷新。</p></body></html>")
			return
		}

		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			http.ServeFile(w, r, filepath.Join(dist, "index.html"))
			return
		}
		fp := filepath.Join(dist, filepath.FromSlash(p))
		if !strings.HasPrefix(fp, filepath.Clean(dist)+string(filepath.Separator)) {
			http.NotFound(w, r)
			return
		}
		if fi, err := os.Stat(fp); err == nil && !fi.IsDir() {
			http.ServeFile(w, r, fp)
			return
		}
		// 客户端路由回退
		http.ServeFile(w, r, filepath.Join(dist, "index.html"))
	})
}

// logRequests 访问日志 + 全局安全响应头
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 全局安全头：禁止内联脚本/外域资源，缓解上传文件触发的存储型 XSS
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self'; object-src 'none'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; base-uri 'self'; frame-ancestors 'self'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("Referrer-Policy", "same-origin")
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, sw.status, time.Since(start).Round(time.Millisecond))
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}
