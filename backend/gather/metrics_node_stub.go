//go:build !linux

package gather

// readSystemOS n'a de sens que sous Linux (syscall.Uname). Stub ailleurs
// pour que le binaire compile sur les machines de dev (macOS).
func readSystemOS() (string, string, string) {
	return "unknown", "unknown", "unknown"
}
