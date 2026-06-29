package a

// Flag is a named boolean type; its underlying type resolves to bool, so fields,
// parameters, and results of type Flag are subject to the naming rule too.
type Flag bool

// Config exercises struct-field boolean naming, including named bool types.
type Config struct {
	IsActive         bool
	HasPremium       bool
	UppercaseEnabled bool
	verbose          bool // want `boolean verbose should use an is/has/can/should/will prefix or an Enabled/Disabled suffix`
	Name             string

	// Named bool type with a bad name: underlying-type resolution must catch it.
	active Flag // want `boolean active should use an is/has/can/should/will prefix or an Enabled/Disabled suffix`
	// Named bool type with a good name: resolved to bool, accepted by the suffix.
	retryEnabled Flag
}

// prefixed exercises the can/should/will predicate prefixes, which must all be
// accepted, and a Disabled-suffixed flag.
func prefixed(canRetry, shouldRun, willStart, featureDisabled bool) bool {
	return canRetry && shouldRun && willStart && featureDisabled
}

// boundary exercises the word-boundary guard: island/hashed/willing merely begin
// with the prefix letters is/has/will but have no boundary, so they are NOT
// exempted and must be flagged. All three diagnostics land on the signature line.
func boundary(island, hashed, willing bool) { // want `boolean island should use an is/has/can/should/will prefix or an Enabled/Disabled suffix` `boolean hashed should use an is/has/can/should/will prefix or an Enabled/Disabled suffix` `boolean willing should use an is/has/can/should/will prefix or an Enabled/Disabled suffix`
	_, _, _ = island, hashed, willing
}

// toggle exercises boolean parameters, including a blank parameter.
func toggle(force bool, _ bool) bool { // want `boolean force should use an is/has/can/should/will prefix or an Enabled/Disabled suffix`
	return force
}

// query exercises a named boolean result.
func query() (ready bool) { // want `boolean ready should use an is/has/can/should/will prefix or an Enabled/Disabled suffix`
	return false
}

// noop exercises a function with no results.
func noop() {}
