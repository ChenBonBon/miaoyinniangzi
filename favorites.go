package main

import (
	"context"
	"errors"

	"miaoyinniangzi/internal/music"
	"miaoyinniangzi/internal/storage"
)

func (a *App) GetFavorites() ([]storage.Song, error) {
	if a.store == nil {
		return nil, errors.New("请在桌面应用中读取收藏")
	}
	songs, err := a.store.Favorites(context.Background())
	if err != nil {
		return nil, errors.New("无法读取收藏，请重试")
	}
	return songs, nil
}
func (a *App) AddFavorite(song storage.Song) error {
	if a.store == nil {
		return errors.New("请在桌面应用中管理收藏")
	}
	_, err := resolveSongInfo(music.Song{ID: song.ID, Source: song.Source, SongID: song.SongID, MediaMid: song.MediaMid, Title: song.Title, Artist: song.Artist, Album: song.Album})
	if err != nil {
		return errors.New("歌曲信息无效，无法收藏")
	}
	if err = a.store.AddFavorite(context.Background(), song); err != nil {
		return errors.New("收藏失败，请重试")
	}
	return nil
}
func (a *App) RemoveFavorite(id string) error {
	if a.store == nil {
		return errors.New("请在桌面应用中管理收藏")
	}
	if len(id) == 0 || len(id) > 64 {
		return errors.New("歌曲标识无效")
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return errors.New("歌曲标识无效")
		}
	}
	if err := a.store.RemoveFavorite(context.Background(), id); err != nil {
		return errors.New("取消收藏失败，请重试")
	}
	return nil
}
