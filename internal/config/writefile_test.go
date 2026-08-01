package config

import "os"

// Test-only indirection kept in its own file so the production package does
// not pull os.WriteFile for nothing.
var osWriteFile = os.WriteFile
