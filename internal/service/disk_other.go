//go:build !unix

package service

func diskSpace(path string) (free, total int64, ok bool) { return 0, 0, false }
