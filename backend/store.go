package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// FileMeta 一条文件记录的元信息
type FileMeta struct {
	ID          string `json:"id"`           // 公开链接用的短 ID
	Name        string `json:"name"`         // 原始文件名
	StoredName  string `json:"stored_name"`  // 存储目录里的落盘名
	Size        int64  `json:"size"`
	Ext         string `json:"ext"`          // 小写后缀（不含点）
	ContentType string `json:"content_type"` // 上传时的 MIME
	UploadedAt  int64  `json:"uploaded_at"`  // Unix 秒
	Downloads   int64  `json:"downloads"`
	DeleteToken string `json:"delete_token"` // 上传者自删口令（敏感，仅在接口内部传递）
	GuestUID    string `json:"guest_uid"`    // 游客 UID（按访客区分，防止冲突）
	Category    string `json:"category"`     // 上传时的分类
}

type Store struct {
	mu    sync.RWMutex
	path  string
	files []FileMeta
}

var store *Store

const metaFile = "data/meta.json"

func InitStore() error {
	store = &Store{path: metaFile}
	if err := os.MkdirAll(filepath.Dir(metaFile), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(absPath(cfg.StorageDir), 0o755); err != nil {
		return err
	}
	data, err := os.ReadFile(metaFile)
	if err != nil {
		if os.IsNotExist(err) {
			return store.save()
		}
		return err
	}
	return json.Unmarshal(data, &store.files)
}

func (s *Store) save() error {
	data, err := json.MarshalIndent(s.files, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(s.path, data)
}

// Add 追加一条记录并持久化
func (s *Store) Add(f FileMeta) error {
	s.mu.Lock()
	s.files = append(s.files, f)
	sort.Slice(s.files, func(i, j int) bool { return s.files[i].UploadedAt > s.files[j].UploadedAt })
	err := s.save()
	s.mu.Unlock()
	return err
}

// Get 按 ID 查找
func (s *Store) Get(id string) (FileMeta, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := range s.files {
		if s.files[i].ID == id {
			return s.files[i], true
		}
	}
	return FileMeta{}, false
}

// GetByToken 按删除口令查找（用于匿名自删）
func (s *Store) GetByToken(token string) (FileMeta, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := range s.files {
		if s.files[i].DeleteToken != "" && s.files[i].DeleteToken == token {
			return s.files[i], true
		}
	}
	return FileMeta{}, false
}

// Delete 删除记录并返回被删元数据
func (s *Store) Delete(id string) (FileMeta, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.files {
		if s.files[i].ID == id {
			f := s.files[i]
			s.files = append(s.files[:i], s.files[i+1:]...)
			_ = s.save()
			return f, true
		}
	}
	return FileMeta{}, false
}

func (s *Store) hasStoredName(name string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := range s.files {
		if s.files[i].StoredName == name {
			return true
		}
	}
	return false
}

// Count 当前文件数
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.files)
}

// All 返回全部记录（倒序，最新在前）
func (s *Store) All() []FileMeta {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]FileMeta, len(s.files))
	copy(out, s.files)
	return out
}

func (s *Store) incDownloads(id string) {
	s.mu.Lock()
	for i := range s.files {
		if s.files[i].ID == id {
			s.files[i].Downloads++
			_ = s.save()
			break
		}
	}
	s.mu.Unlock()
}

// PurgeExpired 清理超过 AutoDeleteDays 的记录并删除磁盘文件，返回删除数量
func PurgeExpired() int {
	if cfg.AutoDeleteDays <= 0 {
		return 0
	}
	cutoff := time.Now().Unix() - int64(cfg.AutoDeleteDays*86400)
	store.mu.Lock()
	kept := store.files[:0]
	removed := 0
	for _, f := range store.files {
		if f.UploadedAt < cutoff {
			os.Remove(filepath.Join(absPath(cfg.StorageDir), f.StoredName))
			removed++
			continue
		}
		kept = append(kept, f)
	}
	store.files = kept
	_ = store.save()
	store.mu.Unlock()
	return removed
}
