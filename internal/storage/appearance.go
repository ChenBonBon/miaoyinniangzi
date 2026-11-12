package storage

import (
	"context"
	"encoding/json"
	"errors"
)

func validAppearance(value string) bool {
	return value == "dark" || value == "light" || value == "system"
}

// Independent state prevents an old playback/download settings draft from
// overwriting the appearance selected in another view.
func (s *Store) Appearance(ctx context.Context) (string, error) {
	payload := json.RawMessage(`"dark"`)
	if err := s.read(ctx, "appearance_v1", &payload); err != nil {
		return "", errors.New("无法读取外观设置")
	}
	var value string
	if err := json.Unmarshal(payload, &value); err != nil || !validAppearance(value) {
		return "", errors.New("无法读取外观设置")
	}
	return value, nil
}
func (s *Store) SaveAppearance(ctx context.Context, value string) error {
	if !validAppearance(value) {
		return errors.New("外观设置无效")
	}
	return s.save(ctx, "appearance_v1", value)
}
