//go:build unix

package main

import "syscall"

// Files created by the server (database, WAL, keys) are private to its user.
func init() { syscall.Umask(0o077) }
