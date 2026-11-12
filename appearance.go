package main

import (
	"context"
	"errors"
	"miaoyinniangzi/internal/appearance"
	"time"
)

func (a *App) GetAppearance() (string, error) {
	if a.store == nil {
		return "", errors.New("外观设置存储不可用")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	value, err := a.store.Appearance(ctx)
	if err == nil && a.ctx != nil {
		appearance.Apply(value)
	}
	return value, err
}
func (a *App) SaveAppearance(value string) error {
	if a.store == nil {
		return errors.New("外观设置存储不可用")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := a.store.SaveAppearance(ctx, value); err != nil {
		return errors.New("无法保存外观设置，请重试")
	}
	if a.ctx != nil {
		appearance.Apply(value)
	}
	return nil
}
