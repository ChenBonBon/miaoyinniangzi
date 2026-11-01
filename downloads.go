package main

import (
	"context"
	"errors"

	"miaoyinniangzi/internal/downloads"
	"miaoyinniangzi/internal/mediastream"
	"miaoyinniangzi/internal/music"
	"miaoyinniangzi/internal/sourcenetwork"
	"miaoyinniangzi/internal/storage"
)

func (a *App) initDownloads() error {
	raw := sourcenetwork.New()
	probe, err := mediastream.New(raw)
	if err != nil {
		return err
	}
	manager, err := downloads.New(a.store, raw, func(ctx context.Context, song storage.Song, quality string) (downloads.Media, error) {
		info, err := resolveSongInfo(music.Song{ID: song.ID, Source: song.Source, SongID: song.SongID, MediaMid: song.MediaMid, Title: song.Title, Artist: song.Artist, Album: song.Album})
		if err != nil {
			return downloads.Media{}, err
		}
		resolved, err := a.sources.ResolveDownload(ctx, info, quality)
		if err != nil {
			return downloads.Media{}, err
		}
		metadata, err := probe.Probe(ctx, resolved.URL)
		if err != nil {
			return downloads.Media{}, err
		}
		return downloads.Media{URL: resolved.URL, Quality: resolved.Quality, Source: resolved.SourceID, Info: metadata, Downgraded: resolved.Downgraded}, nil
	})
	if err != nil {
		probe.Close()
		return errors.New("无法读取下载记录")
	}
	a.downloads = manager
	a.downloadProbe = probe
	return nil
}
func (a *App) closeDownloads() {
	if a.downloads != nil {
		a.downloads.Close()
	}
	if a.downloadProbe != nil {
		a.downloadProbe.Close()
	}
}
func (a *App) GetDownloadTasks() ([]downloads.Task, error) {
	if a.downloads == nil {
		return nil, errors.New("下载服务不可用，原有记录未被清空")
	}
	return a.downloads.List(), nil
}
func (a *App) EnqueueDownloads(songs []music.Song) (downloads.EnqueueResult, error) {
	result := downloads.EnqueueResult{Tasks: []downloads.Task{}}
	if a.downloads == nil || a.store == nil {
		return result, errors.New("请在桌面应用中下载")
	}
	if len(songs) == 0 || len(songs) > downloads.MaxTasks {
		return result, errors.New("请选择 1–10000 首歌曲")
	}
	entries := make([]storage.Song, 0, len(songs))
	for _, song := range songs {
		if _, err := resolveSongInfo(song); err != nil {
			return result, err
		}
		artists := make([]storage.Artist, 0, len(song.Artists))
		for _, artist := range song.Artists {
			artists = append(artists, storage.Artist{ID: artist.ID, Name: artist.Name})
		}
		entries = append(entries, storage.Song{Artists: artists, AlbumMid: song.AlbumMid, ID: song.ID, Source: song.Source, SongID: song.SongID, MediaMid: song.MediaMid, Title: song.Title, Artist: song.Artist, Album: song.Album, CoverURL: song.CoverURL, Duration: song.Duration})
	}
	settings, err := a.store.Settings(context.Background())
	if err != nil {
		return result, errors.New("无法读取下载设置")
	}
	if _, err := a.activeCacheSource(context.Background()); err != nil {
		return result, errors.New("请先在设置中启用一个已校验的音源")
	}
	return a.downloads.Enqueue(entries, settings.DownloadDirectory, settings.DownloadQuality)
}
func (a *App) ControlDownloads(ids []string, action string, all bool) error {
	if a.downloads == nil {
		return errors.New("下载服务不可用")
	}
	return a.downloads.Control(ids, action, all)
}
func (a *App) ContinueDownload(id string) error {
	if a.downloads == nil {
		return errors.New("下载服务不可用")
	}
	return a.downloads.Continue(id)
}
func (a *App) PauseDownload(id string) error {
	if a.downloads == nil {
		return errors.New("下载服务不可用")
	}
	return a.downloads.Pause(id)
}
func (a *App) CancelDownloadBatch(batch string) error {
	if a.downloads == nil {
		return errors.New("下载服务不可用")
	}
	return a.downloads.CancelBatch(batch)
}
func (a *App) RemoveDownload(id string, deleteFile bool) error {
	if a.downloads == nil {
		return errors.New("下载服务不可用")
	}
	return a.downloads.Remove(id, deleteFile)
}
func (a *App) RetryDownloadBatch(batch string) error {
	if a.downloads == nil {
		return errors.New("下载服务不可用")
	}
	for _, task := range a.downloads.List() {
		if task.Batch == batch && task.State == "failed" {
			if err := a.downloads.Continue(task.ID); err != nil {
				return err
			}
		}
	}
	return nil
}
