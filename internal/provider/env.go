package provider

import "os"

func lookupEnv(name string) string { return os.Getenv(name) }
