package middleware

import (
	"testing"
	"time"
)

func TestRateLimiterWindow(t *testing.T) {
	rl := NewRateLimiter(2, time.Minute)
	for i := 0; i < 2; i++ {
		if !rl.Allow("ip-1") {
			t.Fatalf("窗口内第 %d 次应放行", i+1)
		}
	}
	if rl.Allow("ip-1") {
		t.Fatal("超出额度应拒绝")
	}
	if !rl.Allow("ip-2") {
		t.Fatal("不同 key 的额度应互相独立")
	}
}

// 长期运行时，只裁切当前 key 会让不再出现的 IP / 密钥永久留在 map 里。
// 每过一个窗口整体清一次，这里用极短窗口验证清理确实发生。
func TestRateLimiterSweepsStaleKeys(t *testing.T) {
	rl := NewRateLimiter(5, time.Millisecond)
	rl.Allow("stale-key")
	if len(rl.hits) != 1 {
		t.Fatalf("写入后应有 1 个 key，实际 %d", len(rl.hits))
	}
	time.Sleep(5 * time.Millisecond)
	rl.Allow("fresh-key")
	if _, exists := rl.hits["stale-key"]; exists {
		t.Fatal("过期的 key 应被 sweep 清掉，否则会缓慢泄漏内存")
	}
	if _, exists := rl.hits["fresh-key"]; !exists {
		t.Fatal("当前 key 不该被清掉")
	}
}
