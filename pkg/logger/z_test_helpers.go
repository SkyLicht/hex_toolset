package logger

// panicStringer implements fmt.Stringer and panics when String is called.
// Used to test safeSprintf's recover behavior.
type panicStringer struct{}

func (panicStringer) String() string { panic("boom") }
