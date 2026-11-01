//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c -mmacosx-version-min=13.0
#cgo LDFLAGS: -framework Cocoa -framework UniformTypeIdentifiers
#include <Cocoa/Cocoa.h>
#include <dispatch/dispatch.h>
static NSWindow *smokeWindow;
static void rememberWindow(void) {
 if (!smokeWindow) for(NSWindow *window in NSApp.windows) if([window.title isEqualToString:@"Window Lifecycle Smoke"]){smokeWindow=window;break;}
}
static int smokeFlags(void) {
 __block int result=0;
 dispatch_sync(dispatch_get_main_queue(), ^{rememberWindow();if(NSApp.hidden)result|=1;if(smokeWindow.visible)result|=2;if(smokeWindow.miniaturized)result|=4;if(smokeWindow)result|=8;});
 return result;
}
static void smokeAction(int action) {
 dispatch_async(dispatch_get_main_queue(), ^{
  rememberWindow();
  if(action==1)[smokeWindow performClose:nil];
  if(action==2){NSWorkspaceOpenConfiguration *config=[NSWorkspaceOpenConfiguration configuration];config.createsNewApplicationInstance=NO;config.activates=YES;[[NSWorkspace sharedWorkspace]openApplicationAtURL:NSBundle.mainBundle.bundleURL configuration:config completionHandler:^(NSRunningApplication *app,NSError *error){}];}
  if(action==3)[smokeWindow miniaturize:nil];
  if(action==4){NSMenu *menu=NSApp.mainMenu.itemArray.lastObject.submenu;for(NSMenuItem *item in menu.itemArray)if([item.title isEqualToString:@"关闭窗口"]){[NSApp sendAction:item.action to:item.target from:item];break;}}
  if(action==5)[NSApp terminate:nil];
  if(action==6){NSMenu *menu=[NSApp.mainMenu itemAtIndex:0].submenu;for(NSMenuItem *item in menu.itemArray)if([item.keyEquivalent isEqualToString:@"q"]){[NSApp sendAction:item.action to:item.target from:item];break;}}
 });
}
*/
import "C"

func nativeFlags() int        { return int(C.smokeFlags()) }
func nativeAction(action int) { C.smokeAction(C.int(action)) }
