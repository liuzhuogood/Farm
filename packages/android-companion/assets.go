package androidcompanion

import _ "embed"

//go:embed farm-companion.apk
var apk []byte

// APK returns the bundled, signed read-only Android accessibility companion.
func APK() []byte { return apk }
