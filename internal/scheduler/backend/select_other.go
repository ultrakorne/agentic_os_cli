//go:build !darwin && !linux

package backend

func platformBackend(aosHome string) Backend { return nil }
