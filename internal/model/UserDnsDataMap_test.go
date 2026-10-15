package model

import (
	"fmt"
	"sync"
	"testing"

	"github.com/hackliu/DNSLogHub/internal/config"
)

func TestGetAndClear(t *testing.T) {
	d := NewDnsDataMap(config.UserConfig{})

	// 空数据：应返回空且不 panic
	if got := d.GetAndClear("nobody"); len(got) != 0 {
		t.Fatalf("empty GetAndClear returned %d records", len(got))
	}

	d.Set("t", DnsInfo{Subdomain: "a"})
	d.Set("t", DnsInfo{Subdomain: "b"})

	got := d.GetAndClear("t")
	if len(got) != 2 {
		t.Fatalf("got %d records, want 2", len(got))
	}
	// 取后即清：再次读取应为空
	if len(d.Get("t")) != 0 {
		t.Fatal("records should be cleared after GetAndClear")
	}
}

func TestSetConcurrent(t *testing.T) {
	d := NewDnsDataMap(config.UserConfig{})

	var wg sync.WaitGroup
	const n = 100
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			d.Set("t", DnsInfo{Subdomain: fmt.Sprintf("s%d", i)})
		}(i)
	}
	wg.Wait()

	got := d.GetAndClear("t")
	if len(got) != n {
		t.Fatalf("concurrent Set lost records: got %d, want %d", len(got), n)
	}
}
