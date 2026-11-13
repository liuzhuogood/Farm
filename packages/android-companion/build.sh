#!/usr/bin/env bash
set -euo pipefail
companion_root="$(cd "$(dirname "$0")" && pwd)"
companion_sdk="${ANDROID_SDK_ROOT:-${ANDROID_HOME:-$HOME/Library/Android/sdk}}"
companion_tools="$companion_sdk/build-tools/${FARM_ANDROID_BUILD_TOOLS:-35.0.0}"
companion_platform="$companion_sdk/platforms/${FARM_ANDROID_PLATFORM:-android-35}/android.jar"
companion_build="$companion_root/build"
companion_java="${JAVA_HOME:+$JAVA_HOME/bin/}"
for executable in aapt2 d8 zipalign apksigner; do
  test -x "$companion_tools/$executable" || { echo "Missing Android build tool: $companion_tools/$executable" >&2; exit 1; }
done
test -f "$companion_platform" || { echo "Missing Android platform: $companion_platform" >&2; exit 1; }
rm -rf "$companion_build"
mkdir -p "$companion_build/classes" "$companion_build/dex"
"$companion_tools/aapt2" compile --dir "$companion_root/res" -o "$companion_build/resources.zip"
"$companion_tools/aapt2" link -o "$companion_build/resources.apk" --manifest "$companion_root/AndroidManifest.xml" -I "$companion_platform" "$companion_build/resources.zip" --min-sdk-version 26 --target-sdk-version 35
"${companion_java}javac" --release 8 -encoding UTF-8 -classpath "$companion_platform" -d "$companion_build/classes" "$companion_root"/src/app/farm/companion/*.java
mkdir -p "$companion_build/host-test"
"${companion_java}javac" --release 8 -encoding UTF-8 -d "$companion_build/host-test" "$companion_root/src/app/farm/companion/SnapshotEncoding.java" "$companion_root/src/app/farm/companion/CompanionProtocol.java" "$companion_root"/tests/*.java
"${companion_java}java" -cp "$companion_build/host-test" app.farm.companion.SnapshotEncodingTest
"${companion_java}java" -cp "$companion_build/host-test" app.farm.companion.CompanionProtocolTest
"${companion_java}jar" cf "$companion_build/classes.jar" -C "$companion_build/classes" .
"$companion_tools/d8" --min-api 26 --lib "$companion_platform" --output "$companion_build/dex" "$companion_build/classes.jar"
cp "$companion_build/resources.apk" "$companion_build/unsigned.apk"
(cd "$companion_build/dex" && zip -q -X "$companion_build/unsigned.apk" classes.dex)
"$companion_tools/zipalign" -f -p 4 "$companion_build/unsigned.apk" "$companion_build/aligned.apk"
companion_key="${FARM_COMPANION_KEYSTORE:-$HOME/.local/state/farm/android-companion/dev-signing.jks}"
companion_alias="${FARM_COMPANION_KEY_ALIAS:-farm-companion-dev}"
# Android APK signing keys are deployment identity; they never live in this repository.
if [[ -z "${FARM_COMPANION_KEYSTORE:-}" && ! -f "$companion_key" ]]; then
  mkdir -p "$(dirname "$companion_key")"
  chmod 700 "$(dirname "$companion_key")"
  "${companion_java}keytool" -genkeypair -keystore "$companion_key" -storepass changeit -keypass changeit -alias "$companion_alias" -keyalg RSA -keysize 3072 -validity 10000 -dname "CN=Farm Companion Local Development"
  chmod 600 "$companion_key"
fi
if [[ -n "${FARM_COMPANION_KEYSTORE:-}" ]]; then
  : "${FARM_COMPANION_STORE_PASSWORD:?Set release keystore password in FARM_COMPANION_STORE_PASSWORD}"
else
  export FARM_COMPANION_STORE_PASSWORD=changeit
fi
companion_sign_args=(sign --ks "$companion_key" --ks-key-alias "$companion_alias" --ks-pass env:FARM_COMPANION_STORE_PASSWORD)
if [[ -n "${FARM_COMPANION_KEY_PASSWORD:-}" ]]; then companion_sign_args+=(--key-pass env:FARM_COMPANION_KEY_PASSWORD); fi
"$companion_tools/apksigner" "${companion_sign_args[@]}" --out "$companion_root/farm-companion.apk" "$companion_build/aligned.apk"
"$companion_tools/apksigner" verify --verbose "$companion_root/farm-companion.apk"
"$companion_tools/aapt2" dump badging "$companion_root/farm-companion.apk" | head -n 5
shasum -a 256 "$companion_root/farm-companion.apk"
