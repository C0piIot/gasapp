"""Patch Bubblewrap's generated LauncherActivity to ask for location on launch.

androidbrowserhelper's location delegation opens its permission dialog from a
background service, which Android 10+ silently blocks: the page waits forever.
Asking from the launcher, while it is in the foreground, avoids that.
"""

import pathlib
import re
import sys

IMPORTS = """import android.Manifest;
import android.content.pm.PackageManager;
"""

METHODS = """
    @Override
    protected boolean shouldLaunchImmediately() {
        if (checkSelfPermission(Manifest.permission.ACCESS_FINE_LOCATION)
                == PackageManager.PERMISSION_GRANTED) {
            return true;
        }
        requestPermissions(new String[] {
                Manifest.permission.ACCESS_FINE_LOCATION,
                Manifest.permission.ACCESS_COARSE_LOCATION}, 0);
        return false;
    }

    @Override
    public void onRequestPermissionsResult(
            int requestCode, String[] permissions, int[] grantResults) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults);
        launchTwa();
    }
"""

CLASS_DECL = "extends com.google.androidbrowserhelper.trusted.LauncherActivity {"
PACKAGE = re.compile(r"^package [\w.]+;\n", re.MULTILINE)


def patch(source: str) -> str:
    if source.count(CLASS_DECL) != 1 or not PACKAGE.search(source):
        raise ValueError("unexpected LauncherActivity layout; Bubblewrap template changed")
    if "shouldLaunchImmediately" in source:
        raise ValueError("LauncherActivity already overrides shouldLaunchImmediately")
    source = PACKAGE.sub(lambda m: m.group(0) + "\n" + IMPORTS, source, count=1)
    return source.replace(CLASS_DECL, CLASS_DECL + "\n" + METHODS, 1)


if __name__ == "__main__":
    files = list(pathlib.Path(sys.argv[1]).rglob("LauncherActivity.java"))
    if len(files) != 1:
        sys.exit(f"expected one LauncherActivity.java, found {len(files)}")
    files[0].write_text(patch(files[0].read_text()))
    print(f"patched {files[0]}")
