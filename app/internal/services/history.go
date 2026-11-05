package services

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// GenerationRecord 是一条生成历史。
//
// 字段比前端的 GeneratedSvg 多，因为光留 SVG 源码和提示词，回头看历史时
// 根本不知道当时用的什么风格参数 —— 想复现都复现不了。参数一起存下来，
// 才有「照着这条再来一次」的可能。
type GenerationRecord struct {
	ID         string `json:"id"`
	Prompt     string `json:"prompt"`
	Content    string `json:"content"`
	Style      string `json:"style"`
	Color      string `json:"color"`
	Theme      string `json:"theme"`
	Ratio      string `json:"ratio"`
	Complexity string `json:"complexity"`
	Provider   string `json:"provider"`
	Model      string `json:"model"`
	CreatedAt  int64  `json:"createdAt"`
}

// HistoryService 用 SQLite 存生成历史。
//
// 为什么不用 localStorage：那是浏览器存储，上限通常 5~10MB，一张复杂 SVG
// 就几 KB，几百条就撑爆；而且清缓存、换设备全丢。桌面应用有自己的磁盘，
// 没理由受这个限制。
//
// 为什么用 modernc.org/sqlite 而不是 mattn/go-sqlite3：后者要 CGO，
// 交叉编译到 Windows 得配 MinGW 工具链，代价太大。modernc 是纯 Go 实现，
// 直接 go build 就能出各平台二进制。
type HistoryService struct {
	db *sql.DB
}

// maxHistory 是保留条数上限。超了删最旧的 —— 历史是「最近用过的东西」，
// 不是归档，没必要无限增长。
const maxHistory = 500

func NewHistoryService() (*HistoryService, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("拿不到配置目录: %w", err)
	}
	appDir := filepath.Join(dir, "svg-studio")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建目录失败: %w", err)
	}

	dbPath := filepath.Join(appDir, "history.db")
	db, err := sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}

	// SQLite 是单写模型，连接池开太大反而容易撞 SQLITE_BUSY。
	db.SetMaxOpenConns(1)

	s := &HistoryService{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}

	slog.Info("历史库就绪", "path", dbPath)
	return s, nil
}

func (s *HistoryService) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS generations (
	id         TEXT PRIMARY KEY,
	prompt     TEXT NOT NULL DEFAULT '',
	content    TEXT NOT NULL,
	style      TEXT NOT NULL DEFAULT '',
	color      TEXT NOT NULL DEFAULT '',
	theme      TEXT NOT NULL DEFAULT '',
	ratio      TEXT NOT NULL DEFAULT '',
	complexity TEXT NOT NULL DEFAULT '',
	provider   TEXT NOT NULL DEFAULT '',
	model      TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_gen_created ON generations(created_at DESC);
`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("建表失败: %w", err)
	}
	return nil
}

// Save 写入一条历史，并顺手把超出上限的旧记录删掉。
func (s *HistoryService) Save(rec GenerationRecord) *ErrorInfo {
	if s == nil || s.db == nil {
		return &ErrorInfo{Kind: "internal", Message: "历史库不可用。"}
	}
	if rec.ID == "" {
		return &ErrorInfo{Kind: "request", Message: "记录缺少 ID。"}
	}
	if rec.Content == "" {
		return &ErrorInfo{Kind: "request", Message: "记录缺少 SVG 内容。"}
	}
	if rec.CreatedAt == 0 {
		rec.CreatedAt = time.Now().UnixMilli()
	}

	_, err := s.db.Exec(`
INSERT OR REPLACE INTO generations
	(id, prompt, content, style, color, theme, ratio, complexity, provider, model, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.ID, rec.Prompt, rec.Content, rec.Style, rec.Color, rec.Theme,
		rec.Ratio, rec.Complexity, rec.Provider, rec.Model, rec.CreatedAt)
	if err != nil {
		slog.Warn("写历史失败", "err", err)
		return &ErrorInfo{Kind: "io", Message: "保存历史失败：" + err.Error()}
	}

	// 控制总量。用子查询一次搞定，避免先 COUNT 再 DELETE 的竞态。
	if _, err := s.db.Exec(`
DELETE FROM generations WHERE id NOT IN (
	SELECT id FROM generations ORDER BY created_at DESC LIMIT ?
)`, maxHistory); err != nil {
		slog.Debug("裁剪历史失败", "err", err) // 不影响保存本身
	}

	return nil
}

// List 按时间倒序返回历史。limit <= 0 时用默认上限。
func (s *HistoryService) List(limit int) ([]GenerationRecord, *ErrorInfo) {
	if s == nil || s.db == nil {
		return nil, &ErrorInfo{Kind: "internal", Message: "历史库不可用。"}
	}
	if limit <= 0 || limit > maxHistory {
		limit = maxHistory
	}

	rows, err := s.db.Query(`
SELECT id, prompt, content, style, color, theme, ratio, complexity, provider, model, created_at
FROM generations ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, &ErrorInfo{Kind: "io", Message: "读取历史失败：" + err.Error()}
	}
	defer rows.Close()

	out := make([]GenerationRecord, 0, limit)
	for rows.Next() {
		var r GenerationRecord
		if err := rows.Scan(&r.ID, &r.Prompt, &r.Content, &r.Style, &r.Color,
			&r.Theme, &r.Ratio, &r.Complexity, &r.Provider, &r.Model, &r.CreatedAt); err != nil {
			slog.Warn("扫描历史行失败", "err", err)
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

// Delete 删一条。
func (s *HistoryService) Delete(id string) *ErrorInfo {
	if s == nil || s.db == nil {
		return nil
	}
	if id == "" {
		return &ErrorInfo{Kind: "request", Message: "缺少 ID。"}
	}
	if _, err := s.db.Exec(`DELETE FROM generations WHERE id = ?`, id); err != nil {
		return &ErrorInfo{Kind: "io", Message: "删除失败：" + err.Error()}
	}
	return nil
}

// Clear 清空全部历史。
func (s *HistoryService) Clear() *ErrorInfo {
	if s == nil || s.db == nil {
		return nil
	}
	if _, err := s.db.Exec(`DELETE FROM generations`); err != nil {
		return &ErrorInfo{Kind: "io", Message: "清空失败：" + err.Error()}
	}
	return nil
}

// Count 返回当前条数，给界面显示「历史 (N)」用。
func (s *HistoryService) Count() int {
	if s == nil || s.db == nil {
		return 0
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM generations`).Scan(&n); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			slog.Debug("统计历史条数失败", "err", err)
		}
		return 0
	}
	return n
}

// Close 在应用退出时释放数据库句柄。
func (s *HistoryService) Close() {
	if s != nil && s.db != nil {
		s.db.Close()
	}
}
