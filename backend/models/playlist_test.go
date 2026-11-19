package models

import (
	"path/filepath"
	"reflect"
	"testing"
)

func newPlaylistTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := NewDB(filepath.Join(t.TempDir(), "playlist.db"))
	if err != nil {
		t.Fatalf("打开测试库失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func addTestTrack(t *testing.T, db *DB, id, title string) {
	t.Helper()
	if err := db.CreateMusicTrack(&MusicTrack{ID: id, Title: title, Filename: id + ".m4a"}); err != nil {
		t.Fatalf("建测试曲目失败: %v", err)
	}
}

func TestPlaylistCRUD(t *testing.T) {
	db := newPlaylistTestDB(t)
	addTestTrack(t, db, "mus_a", "A")
	addTestTrack(t, db, "mus_b", "B")

	p := &Playlist{ID: NewPlaylistID(), Name: "深夜", Description: "安静一点"}
	// 不存在的 ID 会被过滤；重复 ID 只留一次。
	if err := db.CreatePlaylist(p, []string{"mus_b", "mus_nope", "mus_a", "mus_b"}); err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if p.TrackCount != 2 || !reflect.DeepEqual(p.TrackIDs, []string{"mus_b", "mus_a"}) {
		t.Fatalf("初始曲目 = %v / %d", p.TrackIDs, p.TrackCount)
	}

	list, err := db.ListPlaylists()
	if err != nil || len(list) != 1 || list[0].TrackCount != 2 || len(list[0].TrackIDs) != 0 {
		t.Fatalf("列表 = %+v, err=%v", list, err)
	}

	got, err := db.GetPlaylist(p.ID)
	if err != nil || !reflect.DeepEqual(got.TrackIDs, []string{"mus_b", "mus_a"}) {
		t.Fatalf("详情 = %+v, err=%v", got, err)
	}

	added, err := db.AddPlaylistTracks(p.ID, []string{"mus_a", "mus_b"})
	if err != nil || added != 0 {
		t.Fatalf("重复添加 added=%d err=%v", added, err)
	}
	addTestTrack(t, db, "mus_c", "C")
	added, err = db.AddPlaylistTracks(p.ID, []string{"mus_c"})
	if err != nil || added != 1 {
		t.Fatalf("追加 added=%d err=%v", added, err)
	}
	got, _ = db.GetPlaylist(p.ID)
	if !reflect.DeepEqual(got.TrackIDs, []string{"mus_b", "mus_a", "mus_c"}) {
		t.Fatalf("追加后顺序 = %v", got.TrackIDs)
	}

	if err := db.RemovePlaylistTrack(p.ID, "mus_a"); err != nil {
		t.Fatalf("移除失败: %v", err)
	}
	got, _ = db.GetPlaylist(p.ID)
	if !reflect.DeepEqual(got.TrackIDs, []string{"mus_b", "mus_c"}) {
		t.Fatalf("移除后 = %v", got.TrackIDs)
	}

	updated, err := db.UpdatePlaylist(p.ID, "新名字", "新简介")
	if err != nil || updated.Name != "新名字" || updated.Description != "新简介" {
		t.Fatalf("改名 = %+v, err=%v", updated, err)
	}

	if err := db.DeletePlaylist(p.ID); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if _, err := db.GetPlaylist(p.ID); err == nil {
		t.Fatal("删除后仍可取到详情")
	}
}

// 删除曲目必须同时清掉歌单引用，否则详情里会留下永远解析不到的幽灵 ID。
func TestDeleteTrackCleansPlaylistRefs(t *testing.T) {
	db := newPlaylistTestDB(t)
	addTestTrack(t, db, "mus_x", "X")
	addTestTrack(t, db, "mus_y", "Y")
	p := &Playlist{ID: NewPlaylistID(), Name: "混合"}
	if err := db.CreatePlaylist(p, []string{"mus_x", "mus_y"}); err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if err := db.DeleteMusicTrack("mus_x"); err != nil {
		t.Fatalf("删曲目失败: %v", err)
	}
	got, err := db.GetPlaylist(p.ID)
	if err != nil || !reflect.DeepEqual(got.TrackIDs, []string{"mus_y"}) {
		t.Fatalf("清理后 = %+v, err=%v", got, err)
	}
}
