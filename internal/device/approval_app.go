package device

// ApprovalAppVisual reuses the home screen's exact package mapping. Unknown
// packages have no application icon; a generic icon must not imply identity.
func ApprovalAppVisual(pkg string) (name, icon string) {
	if pkg == "" {
		return "", ""
	}
	name, icon = resolveAppInfo(pkg)
	if icon == "app_default" {
		return "", ""
	}
	return name, icon
}
