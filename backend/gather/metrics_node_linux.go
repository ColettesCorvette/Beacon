//go:build linux

package gather

import "syscall"

func readSystemOS() (string, string, string) {
	var uname syscall.Utsname
	if err := syscall.Uname(&uname); err != nil {
		return "unknown", "unknown", "unknown"
	}
	return int8ToStr(uname.Sysname[:]), int8ToStr(uname.Release[:]), int8ToStr(uname.Version[:])
}

func int8ToStr(arr []int8) string {
	b := make([]byte, 0, len(arr))
	for _, v := range arr {
		if v == 0x00 {
			break
		}
		b = append(b, byte(v))
	}
	return string(b)
}
