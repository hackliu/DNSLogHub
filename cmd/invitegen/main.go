package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/hackliu/DNSLogHub/internal/service"
	"github.com/hackliu/DNSLogHub/internal/store"
)

// invitegen 生成高熵邀请码，并将 bcrypt 哈希写入 SQLite。
// 明文仅在生成时显示一次，用于安全分发给注册用户。
func main() {
	length := flag.Int("len", 26, "邀请码长度（字符数，约 5bit/字符，>=26 提供 >=128bit 熵）")
	uses := flag.Int("uses", 1, "可用次数（-1 表示不限）")
	expires := flag.String("expires", "", "过期时间 RFC3339，空表示不过期")
	bind := flag.String("bind", "", "绑定子域名（空表示不限）")
	note := flag.String("note", "", "备注")
	db := flag.String("db", "data/dnsloghub.db", "SQLite 数据库路径")
	flag.Parse()

	plain, err := service.GenerateInviteCode(*length)
	if err != nil {
		fmt.Fprintln(os.Stderr, "生成失败:", err)
		os.Exit(1)
	}
	hash, err := service.HashInviteCode(plain)
	if err != nil {
		fmt.Fprintln(os.Stderr, "哈希失败:", err)
		os.Exit(1)
	}

	s, err := store.Open(*db)
	if err != nil {
		fmt.Fprintln(os.Stderr, "打开数据库失败:", err)
		os.Exit(1)
	}
	defer s.Close()

	inv := store.Invite{
		Hash:           hash,
		RemainingUses:  *uses,
		ExpiresAt:      *expires,
		BoundSubdomain: *bind,
		Note:           *note,
	}
	if err := s.AddInvite(inv); err != nil {
		fmt.Fprintln(os.Stderr, "写入失败:", err)
		os.Exit(1)
	}

	fmt.Println("邀请码（仅显示一次，请立即保存并安全分发）：")
	fmt.Println("  " + plain)
	fmt.Println("已写入:", *db)
}
