# anonfile · 匿名文件仓库

一个可自托管的匿名文件分享站：访客无需登录即可上传，生成**外链**直接下载；站长可在后台配置**允许的文件类型、单文件大小、文件总数、自动过期**等，并支持**一键备份**与文件列表管理。

下载采用**门槛模式**：点击下载先弹出等待页，顶部为站长配置的**广告位**（无广告则显示 5–20 秒随机倒计时），倒计时结束签发**一次性下载令牌**后才能下载——既防直链绕过，也给站长留了广告位。

技术栈：**Go**（后端，仅标准库，零依赖）+ **Vue 3 / Vite**（前端）。

---

## 功能

**访客端**
- 拖拽/点击上传，可一次选择**最多 5 个文件**批量上传，实时进度
- 自动生成**游客 UID**（浏览器持久化，按访客区分、防止冲突），上传可带**分类**
- 每个文件独立生成外链（`/f/{id}`），可复制、可内联预览、`?dl=1` 强制下载
- **下载门槛**：点击下载弹出等待页，顶部为站长配置的广告位（无广告则随机 5–20 秒倒计时），倒计时结束后才可下载
- **防直链**：强制下载必须携带一次性令牌（门槛页签发、单次有效、3 分钟过期），直接访问 `?dl=1` 返回 403
- 上传/下载/内联预览均有**频率限制**（按 IP 每分钟次数，站长可配，0=不限）
- 返回删除口令，凭口令可匿名自删
- 首页展示最近分享列表（含分类/游客/时间，站长可关闭）

**管理端（`/admin`）**
- 文件管理：搜索、分页、删除、外链（含分类、游客 UID）
- 站点设置：站点名、端口、存储目录、单文件上限、允许类型、总数上限、自动过期天数、开关上传/最近列表、修改密码、上传/下载频率、下载等待秒数、下载页广告代码
- **一键备份**：打包全部文件 + 配置 + 元数据为 zip
- **一键恢复**：上传备份 zip，覆盖式还原文件、配置与元数据

---

## 目录结构

```
anonfile/
├── backend/          Go 后端（仅标准库）
│   ├── main.go       入口 / 路由 / 静态托管 / 访问日志
│   ├── config.go     配置加载与保存（config.json）
│   ├── store.go      文件元数据存储（data/meta.json）
│   ├── handlers.go   上传 / 外链下载 / 公开列表 / 信息
│   ├── admin.go      管理登录 / 文件管理 / 配置 / 备份
│   ├── backup.go     zip 打包
│   └── util.go       随机ID / JSON / 落盘工具
├── frontend/         Vue3 + Vite 前端
│   └── dist/         构建产物（Go 直接托管）
├── run.ps1           一键构建并启动
└── README.md
```

---

## 运行

需要：Go ≥ 1.22，Node ≥ 18（仅首次构建前端时需要）。

**一键运行**（自动构建后端，前端已构建则直接启动）：

```
.\run.ps1
```

或分开执行：

```powershell
# 前端（首次或改过前端代码）
cd frontend
npm install
npm run build

# 后端
cd backend
go build -o anonfile.exe .
.\anonfile.exe
```

启动后访问 **http://127.0.0.1:8080**，管理后台在 **http://127.0.0.1:8080/admin**。

> ⚠️ 默认管理员密码是 **`admin123`**，请登录后第一时间在「设置」里修改。

### 开发模式

前端热更新：`cd frontend && npm run dev`（5173 端口，已配置代理到后端 8080）。

---

## 配置

首次启动在 `backend/config.json` 生成默认配置（也可在管理后台在线修改）：

| 字段 | 说明 | 默认 |
|---|---|---|
| `site_title` | 站点名称 | 匿名文件仓库 |
| `port` | 监听端口 | 8080 |
| `storage_dir` | 文件存储目录 | `./data/files` |
| `frontend_dir` | 前端产物目录 | `../frontend/dist` |
| `max_file_size_mb` | 单文件上限(MB) | 100 |
| `allowed_extensions` | 允许后缀，空=全部 | `[]` |
| `max_files_total` | 文件总数上限，0=不限 | 1000 |
| `upload_enabled` | 是否开放上传 | true |
| `auto_delete_days` | 超过N天自动删除，0=永久 | 0 |
| `public_list` | 首页是否展示最近分享 | true |
| `upload_rate_per_min` | 上传频率（次/分钟/IP，0=不限） | 20 |
| `download_rate_per_min` | 下载频率（次/分钟/IP，0=不限） | 60 |
| `ad_code` | 下载门槛页顶部广告位（HTML，留空=显示倒计时） | "" |
| `wait_sec_min` | 下载等待最短秒数 | 5 |
| `wait_sec_max` | 下载等待最长秒数（范围内随机） | 20 |
| `admin_password` | 管理员密码 | admin123 |

> 存储元数据保存在 `data/meta.json`，与文件一起包含在备份中。

---

## API 概览

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/info` | 站点公开信息（前端渲染提示用） |
| POST | `/api/upload` | 匿名上传（multipart，字段 `file`，可选 `guest_uid`/`category`） |
| GET | `/f/{id}` | 内联预览（支持 Range，限流） |
| GET | `/f/{id}?dl=1&t={token}` | 强制下载（需一次性令牌，否则 403） |
| GET | `/f/{id}/info` | 单文件公开信息 |
| GET | `/api/files` | 分享列表（`q/category/limit/random`） |
| POST | `/api/delete` | 凭删除口令匿名删除 `{token}` |
| POST | `/api/dl/start` | 下载门槛签发一次性令牌 `{id}` → `{token,url}` |
| POST | `/api/admin/login` | 管理员登录 `{password}` → token |
| GET | `/api/admin/files` | 文件列表（搜索/分页，`q/offset/limit`） |
| DELETE | `/api/admin/files/{id}` | 删除文件 |
| GET/PUT | `/api/admin/config` | 读取 / 更新配置（密码留空则不改） |
| POST | `/api/admin/backup` | 一键备份，下载 zip |
| POST | `/api/admin/restore` | 一键恢复：上传备份 zip（multipart，字段 `file`） |

管理接口需请求头 `Authorization: Bearer <token>`。
