package storage

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type DiscoveryChartPreference struct {
	ID     int  `json:"id"`
	Hidden bool `json:"hidden"`
}
type DiscoveryGroupPreference struct {
	Key    string                     `json:"key"`
	Hidden bool                       `json:"hidden"`
	Charts []DiscoveryChartPreference `json:"charts"`
}
type DiscoveryPreferences struct {
	Groups []DiscoveryGroupPreference `json:"groups"`
}

func validateDiscoveryPreferences(value DiscoveryPreferences) error {
	if len(value.Groups) > 32 {
		return errors.New("推荐布局分组过多")
	}
	keys := map[string]bool{}
	total := 0
	for _, group := range value.Groups {
		key := group.Key
		valid := false
		if strings.HasPrefix(key, "qq:") {
			id, err := strconv.Atoi(strings.TrimPrefix(key, "qq:"))
			valid = err == nil && id >= 0 && id <= 1000000 && key == "qq:"+strconv.Itoa(id)
		}
		if strings.HasPrefix(key, "name:") {
			name := strings.TrimPrefix(key, "name:")
			valid = utf8.ValidString(name) && len([]rune(name)) > 0 && len([]rune(name)) <= 128 && !strings.ContainsFunc(name, unicode.IsControl)
		}
		if !valid || keys[key] || len(group.Charts) > 1024 {
			return errors.New("推荐布局分组无效")
		}
		keys[key] = true
		ids := map[int]bool{}
		for _, chart := range group.Charts {
			total++
			if total > 1024 || chart.ID <= 0 || chart.ID > 1000000 || ids[chart.ID] {
				return errors.New("推荐布局榜单无效")
			}
			ids[chart.ID] = true
		}
	}
	return nil
}
func (s *Store) DiscoveryPreferences(ctx context.Context) (DiscoveryPreferences, error) {
	value := DiscoveryPreferences{Groups: []DiscoveryGroupPreference{}}
	if err := s.read(ctx, "discovery_preferences", &value); err != nil {
		return DiscoveryPreferences{}, errors.New("无法读取推荐布局")
	}
	if err := validateDiscoveryPreferences(value); err != nil {
		return DiscoveryPreferences{}, err
	}
	if value.Groups == nil {
		value.Groups = []DiscoveryGroupPreference{}
	}
	for i := range value.Groups {
		if value.Groups[i].Charts == nil {
			value.Groups[i].Charts = []DiscoveryChartPreference{}
		}
	}
	return value, nil
}
func (s *Store) SaveDiscoveryPreferences(ctx context.Context, value DiscoveryPreferences) error {
	if err := validateDiscoveryPreferences(value); err != nil {
		return err
	}
	if value.Groups == nil {
		value.Groups = []DiscoveryGroupPreference{}
	}
	for i := range value.Groups {
		if value.Groups[i].Charts == nil {
			value.Groups[i].Charts = []DiscoveryChartPreference{}
		}
	}
	return s.save(ctx, "discovery_preferences", value)
}
