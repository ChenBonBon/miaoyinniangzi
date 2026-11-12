#import <Cocoa/Cocoa.h>

void miaoyinAppearance(int mode) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (NSApp == nil) return;
        NSApp.appearance = mode == 1 ? [NSAppearance appearanceNamed:NSAppearanceNameDarkAqua]
            : mode == 2 ? [NSAppearance appearanceNamed:NSAppearanceNameAqua] : nil;
    });
}
