//go:build darwin

package backend

func platformBackend(aosHome string) Backend {
	return NewLaunchd(aosHome)
}
