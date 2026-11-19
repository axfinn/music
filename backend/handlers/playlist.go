package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/axfinn/music/backend/models"

	"github.com/gin-gonic/gin"
)

// 公开歌单（管理员发布，访客可读可播）的 HTTP 层。
// 访客自己的歌单在浏览器 localStorage，不经过这里。
// 路由的权限门在 routes/music.go：读公开，写必须管理会话。

const (
	playlistNameMax        = 80
	playlistDescriptionMax = 500
	playlistBatchTracksMax = 500
)

type playlistWriteReq struct {
	Name        *string  `json:"name"`
	Description *string  `json:"description"`
	TrackIDs    []string `json:"track_ids"`
}

func playlistReqError(err error) (int, string) {
	if errors.Is(err, sql.ErrNoRows) {
		return http.StatusNotFound, "歌单不存在"
	}
	return http.StatusInternalServerError, "操作失败: " + err.Error()
}

func bindPlaylistReq(c *gin.Context) (playlistWriteReq, bool) {
	var req playlistWriteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式不正确"})
		return req, false
	}
	if req.Name != nil {
		n := strings.TrimSpace(*req.Name)
		if n == "" || len([]rune(n)) > playlistNameMax {
			c.JSON(http.StatusBadRequest, gin.H{"error": "歌单名称需为 1~80 个字符"})
			return req, false
		}
		req.Name = &n
	}
	if req.Description != nil {
		d := strings.TrimSpace(*req.Description)
		if len([]rune(d)) > playlistDescriptionMax {
			c.JSON(http.StatusBadRequest, gin.H{"error": "歌单简介不能超过 500 个字符"})
			return req, false
		}
		req.Description = &d
	}
	if len(req.TrackIDs) > playlistBatchTracksMax {
		c.JSON(http.StatusBadRequest, gin.H{"error": "单次最多添加 500 首"})
		return req, false
	}
	return req, true
}

// ListPlaylists GET /api/music/playlists — 访客可读
func (h *MusicHandler) ListPlaylists(c *gin.Context) {
	items, err := h.db.ListPlaylists()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败: " + err.Error()})
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// GetPlaylist GET /api/music/playlists/:id — 访客可读
func (h *MusicHandler) GetPlaylist(c *gin.Context) {
	p, err := h.db.GetPlaylist(c.Param("id"))
	if err != nil {
		status, msg := playlistReqError(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"item": p})
}

// CreatePlaylist POST /api/music/playlists — 管理
func (h *MusicHandler) CreatePlaylist(c *gin.Context) {
	req, ok := bindPlaylistReq(c)
	if !ok {
		return
	}
	if req.Name == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请填写歌单名称"})
		return
	}
	desc := ""
	if req.Description != nil {
		desc = *req.Description
	}
	p := &models.Playlist{ID: models.NewPlaylistID(), Name: *req.Name, Description: desc}
	if err := h.db.CreatePlaylist(p, req.TrackIDs); err != nil {
		status, msg := playlistReqError(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusOK, gin.H{"item": p})
}

// UpdatePlaylist PATCH /api/music/playlists/:id — 管理
func (h *MusicHandler) UpdatePlaylist(c *gin.Context) {
	req, ok := bindPlaylistReq(c)
	if !ok {
		return
	}
	if req.Name == nil && req.Description == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "没有要修改的内容"})
		return
	}
	old, err := h.db.GetPlaylist(c.Param("id"))
	if err != nil {
		status, msg := playlistReqError(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	name, desc := old.Name, old.Description
	if req.Name != nil {
		name = *req.Name
	}
	if req.Description != nil {
		desc = *req.Description
	}
	p, err := h.db.UpdatePlaylist(old.ID, name, desc)
	if err != nil {
		status, msg := playlistReqError(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusOK, gin.H{"item": p})
}

// DeletePlaylist DELETE /api/music/playlists/:id — 管理
func (h *MusicHandler) DeletePlaylist(c *gin.Context) {
	if err := h.db.DeletePlaylist(c.Param("id")); err != nil {
		status, msg := playlistReqError(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// AddPlaylistTracks POST /api/music/playlists/:id/tracks — 管理
func (h *MusicHandler) AddPlaylistTracks(c *gin.Context) {
	req, ok := bindPlaylistReq(c)
	if !ok {
		return
	}
	if len(req.TrackIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请选择要添加的曲目"})
		return
	}
	added, err := h.db.AddPlaylistTracks(c.Param("id"), req.TrackIDs)
	if err != nil {
		status, msg := playlistReqError(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusOK, gin.H{"added": added})
}

// RemovePlaylistTrack DELETE /api/music/playlists/:id/tracks/:trackId — 管理
func (h *MusicHandler) RemovePlaylistTrack(c *gin.Context) {
	if err := h.db.RemovePlaylistTrack(c.Param("id"), c.Param("trackId")); err != nil {
		status, msg := playlistReqError(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
