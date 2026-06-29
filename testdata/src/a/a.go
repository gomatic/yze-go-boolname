package a

// Config exercises struct-field boolean naming.
type Config struct {
	IsActive         bool
	HasPremium       bool
	UppercaseEnabled bool
	verbose          bool // want `boolean verbose`
	Name             string
}

// toggle exercises boolean parameters, including a blank parameter.
func toggle(force bool, _ bool) bool { // want `boolean force`
	return force
}

// query exercises a named boolean result.
func query() (ready bool) { // want `boolean ready`
	return false
}

// noop exercises a function with no results.
func noop() {}
