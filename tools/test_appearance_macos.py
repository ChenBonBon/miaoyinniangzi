#!/usr/bin/env python3
"""Probe our AppKit theme implementation in an isolated process, no user profile."""
import json
from pathlib import Path
import subprocess
import sys
import tempfile


def main():
    if sys.platform != "darwin":
        raise SystemExit("macOS is required")
    repo = Path(__file__).resolve().parents[1]
    source = r'''
#import <Cocoa/Cocoa.h>
void miaoyinAppearance(int mode);
static void drain(void) {
    [[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:0.1]];
}
int main(void) {
    @autoreleasepool {
        [NSApplication sharedApplication];
        [NSApp setActivationPolicy:NSApplicationActivationPolicyProhibited];
        miaoyinAppearance(1); drain();
        BOOL dark = [NSApp.appearance.name isEqualToString:NSAppearanceNameDarkAqua];
        miaoyinAppearance(2); drain();
        BOOL light = [NSApp.appearance.name isEqualToString:NSAppearanceNameAqua];
        miaoyinAppearance(0); drain();
        BOOL system = NSApp.appearance == nil;
        printf("{\"dark\":%s,\"light\":%s,\"system\":%s}\n",
               dark ? "true" : "false", light ? "true" : "false", system ? "true" : "false");
        return dark && light && system ? 0 : 1;
    }
}
'''
    with tempfile.TemporaryDirectory(prefix="miaoyin-appearance-smoke-") as directory:
        root = Path(directory)
        probe = root / "probe.m"
        probe.write_text(source)
        binary = root / "probe"
        subprocess.run(["clang", "-fblocks", "-mmacosx-version-min=13.0", "-framework", "Cocoa",
                        str(probe), str(repo / "internal/appearance/native_darwin.m"),
                        "-o", str(binary)], check=True, capture_output=True)
        result = subprocess.run([str(binary)], capture_output=True, text=True, timeout=10)
        reports = [line for line in result.stdout.splitlines() if line.startswith("{")]
        if result.returncode or not reports or not all(json.loads(reports[-1]).values()):
            raise SystemExit("isolated AppKit appearance probe failed")
        print(reports[-1])


if __name__ == "__main__":
    main()
