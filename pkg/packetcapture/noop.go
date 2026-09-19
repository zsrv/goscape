package packetcapture

// NoopCapture returns a disabled-mode Capture whose methods are all
// short-circuits. Equivalent to `New(CaptureOpts{Enabled: false})`; provided
// as a convenience for call sites that want an always-noop instance without
// passing a config.
func NoopCapture() *Capture { return &Capture{enabled: false} }
