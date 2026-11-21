package store_test

import "os"

func mkdir(p string) error { return os.MkdirAll(p, 0o755) }
