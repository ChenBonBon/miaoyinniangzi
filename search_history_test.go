package main

import (
	"path/filepath"
	"testing"

	"miaoyinniangzi/internal/storage"
)

func TestSearchHistoryBridgePersistsAndReturnsSnapshots(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.Open(dir, filepath.Join(dir, "Downloads"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app := &App{store: store}
	entries, err := app.RecordSearchHistory(" 周杰伦 ")
	if err != nil || len(entries) != 1 || entries[0] != "周杰伦" {
		t.Fatalf("record: %v %v", entries, err)
	}
	entries, err = app.GetSearchHistory()
	if err != nil || len(entries) != 1 {
		t.Fatal("read failed")
	}
	entries, err = app.RemoveSearchHistory("周杰伦")
	if err != nil || entries == nil || len(entries) != 0 {
		t.Fatal("remove failed")
	}
	if _, err := app.RecordSearchHistory("林俊杰"); err != nil {
		t.Fatal(err)
	}
	entries, err = app.ClearSearchHistory()
	if err != nil || len(entries) != 0 {
		t.Fatal("clear failed")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := app.GetSearchHistory(); err == nil || err.Error() != "无法读取搜索历史，请重试" {
		t.Fatal("database error was not sanitized")
	}
	if _, err := (&App{}).GetSearchHistory(); err == nil {
		t.Fatal("accepted history without database")
	}
}
