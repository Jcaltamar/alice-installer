package compose

// ComposeFiles returns the ordered list of compose file paths to pass via -f flags.
//
// When gpuDetected is true, overlayPath is appended after baselinePath so that
// the GPU service extensions override the base configuration.
// When gpuDetected is false, only baselinePath is returned.
func ComposeFiles(gpuDetected bool, baselinePath, overlayPath string) []string {
	if gpuDetected {
		return []string{baselinePath, overlayPath}
	}
	return []string{baselinePath}
}
