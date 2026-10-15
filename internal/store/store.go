package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/hackliu/DNSLogHub/internal/model"

	_ "modernc.org/sqlite"
)

// User 用户（注册用户，含启用状态）
type User struct {
	Token     string
	Subdomain string
	Domain    string // 空 = 主域名
	Enabled   bool
	CreatedAt int64
}

// Invite 邀请码存储态（仅哈希与元数据）
type Invite struct {
	ID             int64
	Hash           string
	RemainingUses  int
	ExpiresAt      string
	BoundSubdomain string
	Note           string
	CreatedAt      int64
}

// RecordEntry 待写入的一条外带记录
type RecordEntry struct {
	Token string
	Info  model.DnsInfo
}

// Store SQLite 持久化存储
type Store struct {
	db *sql.DB
}

// 版本化迁移：migrations[0] 对应 user_version=1
var migrations = []string{
	`CREATE TABLE IF NOT EXISTS users (
		token       TEXT PRIMARY KEY,
		subdomain   TEXT NOT NULL UNIQUE,
		enabled     INTEGER NOT NULL DEFAULT 1,
		created_at  INTEGER NOT NULL
	);
	CREATE TABLE IF NOT EXISTS invites (
		id              INTEGER PRIMARY KEY AUTOINCREMENT,
		hash            TEXT NOT NULL,
		remaining_uses  INTEGER NOT NULL,
		expires_at      TEXT NOT NULL DEFAULT '',
		bound_subdomain TEXT NOT NULL DEFAULT '',
		note            TEXT NOT NULL DEFAULT '',
		created_at      INTEGER NOT NULL
	);
	CREATE TABLE IF NOT EXISTS records (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		token       TEXT NOT NULL,
		type        TEXT NOT NULL,
		subdomain   TEXT NOT NULL,
		ipaddress   TEXT NOT NULL,
		ip_location TEXT NOT NULL DEFAULT '',
		request     TEXT NOT NULL DEFAULT '',
		time        INTEGER NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_records_token_time ON records(token, time);`,
	`ALTER TABLE users ADD COLUMN domain TEXT NOT NULL DEFAULT '';`,
}

// Open 打开（不存在则创建）SQLite 数据库，启用 WAL / busy_timeout / 外键。
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, err
	}
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// SQLite 单写者：限制单连接，避免 "database is locked"
	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.Migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

// Migrate 按 user_version 执行版本化迁移（事务内，失败回滚）。
func (s *Store) Migrate() error {
	var ver int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&ver); err != nil {
		return err
	}
	for i, m := range migrations[ver:] {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(m); err != nil {
			_ = tx.Rollback()
			return err
		}
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", ver+i+1)); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// Backup 在线备份到独立文件（VACUUM INTO）。
func (s *Store) Backup(dest string) error {
	_, err := s.db.Exec(fmt.Sprintf("VACUUM INTO '%s'", dest))
	return err
}

// ---------- records ----------

// InsertRecords 事务批量写入记录。
func (s *Store) InsertRecords(entries []RecordEntry) error {
	if len(entries) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO records(token, type, subdomain, ipaddress, ip_location, request, time) VALUES(?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, e := range entries {
		if _, err := stmt.Exec(e.Token, string(e.Info.Type), e.Info.Subdomain, e.Info.Ipaddress, e.Info.IpLocation, e.Info.Request, e.Info.Time); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RecordsByToken 按 token 读取全部记录（按写入顺序）。
func (s *Store) RecordsByToken(token string) ([]model.DnsInfo, error) {
	rows, err := s.db.Query(`SELECT type, subdomain, ipaddress, ip_location, request, time FROM records WHERE token = ? ORDER BY id`, token)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.DnsInfo
	for rows.Next() {
		var d model.DnsInfo
		var typ string
		if err := rows.Scan(&typ, &d.Subdomain, &d.Ipaddress, &d.IpLocation, &d.Request, &d.Time); err != nil {
			return nil, err
		}
		d.Type = model.DnsType(typ)
		out = append(out, d)
	}
	return out, rows.Err()
}

// ClearRecords 删除某用户全部记录。
func (s *Store) ClearRecords(token string) error {
	_, err := s.db.Exec(`DELETE FROM records WHERE token = ?`, token)
	return err
}

// ---------- users ----------

func (s *Store) SaveUser(u User) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO users(token, subdomain, domain, enabled, created_at) VALUES(?,?,?,?,?)`,
		u.Token, u.Subdomain, u.Domain, boolToInt(u.Enabled), u.CreatedAt)
	return err
}

func (s *Store) ListUsers() ([]User, error) {
	rows, err := s.db.Query(`SELECT token, subdomain, domain, enabled, created_at FROM users ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		var enabled int
		if err := rows.Scan(&u.Token, &u.Subdomain, &u.Domain, &enabled, &u.CreatedAt); err != nil {
			return nil, err
		}
		u.Enabled = enabled != 0
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) UpdateUserSubdomain(token, subdomain string) error {
	_, err := s.db.Exec(`UPDATE users SET subdomain = ? WHERE token = ?`, subdomain, token)
	return err
}

// UpdateUserDomain 更新用户所属域名。
func (s *Store) UpdateUserDomain(token, domain string) error {
	_, err := s.db.Exec(`UPDATE users SET domain = ? WHERE token = ?`, domain, token)
	return err
}

func (s *Store) SetUserEnabled(token string, enabled bool) error {
	_, err := s.db.Exec(`UPDATE users SET enabled = ? WHERE token = ?`, boolToInt(enabled), token)
	return err
}

// GetUser 按 token 读取用户。
func (s *Store) GetUser(token string) (User, bool, error) {
	var u User
	var enabled int
	err := s.db.QueryRow(`SELECT token, subdomain, domain, enabled, created_at FROM users WHERE token = ?`, token).
		Scan(&u.Token, &u.Subdomain, &u.Domain, &enabled, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return u, false, nil
	}
	if err != nil {
		return u, false, err
	}
	u.Enabled = enabled != 0
	return u, true, nil
}

// ResetUserToken 在事务中更新用户 token（删除旧行 + 插入新行，保留其余字段）。
func (s *Store) ResetUserToken(oldToken, newToken string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var subdomain, domain string
	var enabled int
	var createdAt int64
	if err := tx.QueryRow(`SELECT subdomain, domain, enabled, created_at FROM users WHERE token = ?`, oldToken).
		Scan(&subdomain, &domain, &enabled, &createdAt); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM users WHERE token = ?`, oldToken); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO users(token, subdomain, domain, enabled, created_at) VALUES(?,?,?,?,?)`, newToken, subdomain, domain, enabled, createdAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DeleteUser(token string) error {
	_, err := s.db.Exec(`DELETE FROM users WHERE token = ?`, token)
	return err
}

// ---------- invites ----------

func (s *Store) AddInvite(inv Invite) error {
	_, err := s.db.Exec(`INSERT INTO invites(hash, remaining_uses, expires_at, bound_subdomain, note, created_at) VALUES(?,?,?,?,?,?)`,
		inv.Hash, inv.RemainingUses, inv.ExpiresAt, inv.BoundSubdomain, inv.Note, time.Now().Unix())
	return err
}

func (s *Store) ListInvites() ([]Invite, error) {
	rows, err := s.db.Query(`SELECT id, hash, remaining_uses, expires_at, bound_subdomain, note, created_at FROM invites ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Invite
	for rows.Next() {
		var v Invite
		if err := rows.Scan(&v.ID, &v.Hash, &v.RemainingUses, &v.ExpiresAt, &v.BoundSubdomain, &v.Note, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ConsumeInvite 扣减一次使用次数（remaining_uses > 0 时）。
func (s *Store) ConsumeInvite(hash string) error {
	_, err := s.db.Exec(`UPDATE invites SET remaining_uses = remaining_uses - 1 WHERE hash = ? AND remaining_uses > 0`, hash)
	return err
}

// RevokeInvite 作废邀请码（置为 0 次）。
func (s *Store) RevokeInvite(id int64) error {
	_, err := s.db.Exec(`UPDATE invites SET remaining_uses = 0 WHERE id = ?`, id)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
