package gui

// ng_low_latency (sdl.c) sets the Metal layer's drawable count through the
// Objective-C runtime. Link the runtime explicitly: with Go 1.27, nothing
// else brought in objc_msgSend and the link failed.

// #cgo LDFLAGS: -lobjc
import "C"
