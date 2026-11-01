package main

import (
	"context"
	"errors"
)

func (a *App) GetSearchHistory() ([]string, error) {
	if a.store == nil {
		return nil, errors.New("本地搜索历史尚未就绪")
	}
	result, err := a.store.SearchHistory(context.Background())
	if err != nil {
		return nil, errors.New("无法读取搜索历史，请重试")
	}
	return result, nil
}
func (a *App) RecordSearchHistory(query string) ([]string, error) {
	if a.store == nil {
		return nil, errors.New("本地搜索历史尚未就绪")
	}
	result, err := a.store.RecordSearchHistory(context.Background(), query)
	if err != nil {
		return nil, errors.New("无法保存搜索历史，不影响本次搜索")
	}
	return result, nil
}
func (a *App) RemoveSearchHistory(query string) ([]string, error) {
	if a.store == nil {
		return nil, errors.New("本地搜索历史尚未就绪")
	}
	result, err := a.store.RemoveSearchHistory(context.Background(), query)
	if err != nil {
		return nil, errors.New("无法删除搜索历史，请重试")
	}
	return result, nil
}
func (a *App) ClearSearchHistory() ([]string, error) {
	if a.store == nil {
		return nil, errors.New("本地搜索历史尚未就绪")
	}
	result, err := a.store.ClearSearchHistory(context.Background())
	if err != nil {
		return nil, errors.New("无法清空搜索历史，请重试")
	}
	return result, nil
}
