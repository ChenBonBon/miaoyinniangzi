package main

import (
	"context"
	"errors"
	"miaoyinniangzi/internal/storage"
)

func (a *App) GetDiscoveryPreferences() (storage.DiscoveryPreferences, error) {
	if a.store == nil {
		return storage.DiscoveryPreferences{}, errors.New("推荐布局存储不可用")
	}
	return a.store.DiscoveryPreferences(context.Background())
}
func (a *App) SaveDiscoveryPreferences(value storage.DiscoveryPreferences) error {
	if a.store == nil {
		return errors.New("推荐布局存储不可用")
	}
	if err := a.store.SaveDiscoveryPreferences(context.Background(), value); err != nil {
		return errors.New("无法保存推荐布局，请重试")
	}
	return nil
}
