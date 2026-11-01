// Package windowlife keeps native close-to-hide policy separate from quitting.
package windowlife

import (
	"context"
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	"github.com/wailsapp/wails/v2/pkg/options"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"runtime"
)

func Configure(app *options.App, currentContext func() context.Context, beforeQuit func(context.Context) bool) {
	if runtime.GOOS != "darwin" {
		return
	}
	// NSWindowDelegate hides the app without entering the quit path. The native
	// window and WebView survive; Cocoa handles subsequent Dock activation.
	app.HideWindowOnClose = true
	app.OnBeforeClose = beforeQuit
	// Keep the native app/edit menus, including the real Cmd+Q quit action.
	app.Menu = menu.NewMenuFromItems(menu.AppMenu(), menu.EditMenu())
	window := app.Menu.AddSubmenu("窗口")
	window.AddText("显示主窗口", keys.CmdOrCtrl("0"), func(*menu.CallbackData) {
		if ctx := currentContext(); ctx != nil {
			wailsruntime.Show(ctx)
			wailsruntime.WindowUnminimise(ctx)
			wailsruntime.WindowShow(ctx)
		}
	})
	window.AddText("关闭窗口", keys.CmdOrCtrl("w"), func(*menu.CallbackData) {
		if ctx := currentContext(); ctx != nil {
			wailsruntime.Hide(ctx)
		}
	})
	window.AddText("最小化", keys.CmdOrCtrl("m"), func(*menu.CallbackData) {
		if ctx := currentContext(); ctx != nil {
			wailsruntime.WindowMinimise(ctx)
		}
	})
}
