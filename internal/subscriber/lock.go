// Package subscriber coordinates subscriber database mutations within the server.
package subscriber

import "sync"

// Mutex serializes database replacement, SIM programming and subscriber appends.
// Hold it from preflight validation until the corresponding mutation is complete.
// It does not coordinate writes made by external processes.
var Mutex sync.Mutex
