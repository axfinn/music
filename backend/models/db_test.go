package models

import (
	"testing"
	"time"
)

// 历史库里出现过的四种时间写法都要能读，否则一条脏记录能让整页列表 500。
func TestTSParseLegacyFormats(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		want   string // 期望的本地墙钟（不含时区），空表示只看能不能解析
		wantOK bool
	}{
		{"本项目规范格式", "2026-11-10 15:30:06", "2026-11-10 15:30:06", true},
		{"历史库原文", "2026-11-10 15:30:06.560582875+08:00", "2026-11-10 15:30:06", true},
		{"Go String 时区重复", "2026-11-10 15:30:06.560582875 +0800 +0800", "2026-11-10 15:30:06", true},
		{"RFC3339 UTC", "2026-11-10T07:30:06Z", "2026-11-10 15:30:06", true},
		{"纯日期", "2026-11-10", "2026-11-10 00:00:00", true},
		{"负数时区", "2026-11-10 02:30:06-05:00", "2026-11-10 15:30:06", true},
		{"空字符串", "", "", true},
		{"无法解析", "昨天下午", "", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var ts TS
			err := ts.parse(c.input)
			if c.wantOK != (err == nil) {
				t.Fatalf("parse(%q) err=%v，期望成功=%v", c.input, err, c.wantOK)
			}
			if !c.wantOK {
				return
			}
			if c.want == "" {
				return
			}
			// 统一换到本地时区再比墙钟，避免测试机器时区不同导致误判。
			got := ts.Time().In(time.Local).Format(tsLayout)
			if got != c.want {
				t.Fatalf("parse(%q) = %s，期望 %s", c.input, got, c.want)
			}
		})
	}
}

// 纳秒精度不能被截掉：同一秒内导入的几千条歌词要靠它排序。
func TestTSParseKeepsNanoseconds(t *testing.T) {
	var ts TS
	if err := ts.parse("2026-11-10 15:30:06.560582875+08:00"); err != nil {
		t.Fatalf("parse 失败: %v", err)
	}
	if got := ts.Time().Nanosecond(); got != 560582875 {
		t.Fatalf("纳秒 = %d，期望 560582875", got)
	}
}
