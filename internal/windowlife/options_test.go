package windowlife

import (
	"context"
	"github.com/wailsapp/wails/v2/pkg/options"
	"runtime"
	"testing"
)

func TestMacClosePolicyDoesNotReplaceStartupOrTurnQuitIntoHide(t *testing.T) {
	started := false
	quit := false
	app := &options.App{OnStartup: func(context.Context) { started = true }}
	Configure(app, func() context.Context { return nil }, func(context.Context) bool { quit = true; return false })
	app.OnStartup(context.Background())
	if !started {
		t.Fatal("startup lost")
	}
	if runtime.GOOS != "darwin" {
		if app.HideWindowOnClose || app.OnBeforeClose != nil {
			t.Fatal("non-mac behavior changed")
		}
		return
	}
	if !app.HideWindowOnClose || app.Menu == nil || app.OnBeforeClose == nil {
		t.Fatal("missing mac native close policy")
	}
	if app.OnBeforeClose(context.Background()) || !quit {
		t.Fatal("real quit prevented")
	}
	window := app.Menu.Items[len(app.Menu.Items)-1]
	if window.Label != "窗口" || len(window.SubMenu.Items) != 3 {
		t.Fatal("window recovery commands missing")
	}
}
