#!/usr/bin/env python3
"""Validate the shipped APK rather than assuming successful compilation is enough."""
import os
import re
from pathlib import Path
import subprocess

root = Path(__file__).resolve().parent
sdk = Path(os.environ.get("ANDROID_SDK_ROOT", os.environ.get("ANDROID_HOME", str(Path.home() / "Library/Android/sdk"))))
tools = sdk / "build-tools" / os.environ.get("FARM_ANDROID_BUILD_TOOLS", "35.0.0")
apk = root / "farm-companion.apk"

def run(name, *args):
    return subprocess.check_output([str(tools / name), *args], text=True)

signature = run("apksigner", "verify", "--verbose", str(apk))
assert "Verified using v2 scheme (APK Signature Scheme v2): true" in signature
badging = run("aapt2", "dump", "badging", str(apk))
assert "package: name='app.farm.companion' versionCode='2' versionName='1.0.1'" in badging
assert "minSdkVersion:'26'" in badging
assert "targetSdkVersion:'35'" in badging
permissions = run("aapt2", "dump", "permissions", str(apk))
assert "uses-permission" not in permissions, permissions
manifest = run("aapt2", "dump", "xmltree", "--file", "AndroidManifest.xml", str(apk))
assert "android.permission.BIND_ACCESSIBILITY_SERVICE" in manifest
assert "app.farm.companion.FarmAccessibilityService" in manifest or ".FarmAccessibilityService" in manifest
service = run("aapt2", "dump", "xmltree", "--file", "res/xml/accessibility_service.xml", str(apk))
assert re.search(r"canRetrieveWindowContent\([^)]*\)=true", service), service
assert "canPerformGestures" not in service
assert "canTakeScreenshot" not in service
assert "canRequestFilterKeyEvents" not in service
print("APK verified: app.farm.companion 1.0.1 (code 2), API 26–35, signed, read-only accessibility, no requested permissions")
