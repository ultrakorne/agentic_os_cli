//go:build linux

package backend

func platformBackend(aosHome string) Backend {
	return NewSystemd(aosHome)
}
