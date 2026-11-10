package main

import (
	"testing"
	"time"

	"github.com/axfinn/music/backend/models"
)

// 迁移时驱动会把 DATETIME 文本读成 time.Time，原样写回就变成
// "… +0800 +0800"，本项目解析不了。normalizeValue 必须落成可解析的文本。
func TestNormalizeValueFormatsTime(t *testing.T) {
	tm := time.Date(2026, 10, 30, 17, 42, 40, 294064882, time.FixedZone("", 8*3600))
	got, ok := normalizeValue(tm).(string)
	if !ok {
		t.Fatalf("time.Time 应被转成字符串，实际 %T", normalizeValue(tm))
	}
	if got != "2026-10-30 17:42:40.294064882+08:00" {
		t.Fatalf("格式化结果 = %q", got)
	}

	// 非时间值原样透传。
	if v := normalizeValue(int64(42)); v != int64(42) {
		t.Fatalf("整型不该被改动: %v", v)
	}
	if v := normalizeValue("标题"); v != "标题" {
		t.Fatalf("字符串不该被改动: %v", v)
	}
}

// 迁移出来的文本必须能被项目的时间类型读回来（这是当初 500 的根因）。
func TestNormalizedTimeIsParsable(t *testing.T) {
	tm := time.Date(2026, 10, 30, 17, 42, 40, 294064882, time.FixedZone("", 8*3600))
	s, _ := normalizeValue(tm).(string)

	var ts models.TS
	if err := ts.Scan(s); err != nil {
		t.Fatalf("迁移写入的时间读不回来: %v", err)
	}
	if !ts.Time().Equal(tm) {
		t.Fatalf("读回时间 %v，期望 %v", ts.Time(), tm)
	}
}
