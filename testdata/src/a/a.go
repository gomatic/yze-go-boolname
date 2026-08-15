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
// exempted and must each be flagged on their own parameter line.
func boundary(
	island bool, // want `boolean island should use an is/has/can/should/will prefix or an Enabled/Disabled suffix`
	hashed bool, // want `boolean hashed should use an is/has/can/should/will prefix or an Enabled/Disabled suffix`
	willing bool, // want `boolean willing should use an is/has/can/should/will prefix or an Enabled/Disabled suffix`
) {
	_, _, _ = island, hashed, willing
}

// toggle exercises boolean parameters, including a blank parameter.
func toggle(
	force bool, // want `boolean force should use an is/has/can/should/will prefix or an Enabled/Disabled suffix`
	_ bool,
) bool {
	return force
}

// query exercises a named boolean result.
func query() (ready bool) { // want `boolean ready should use an is/has/can/should/will prefix or an Enabled/Disabled suffix`
	return false
}

// unicodeBoundary exercises non-ASCII word boundaries after a predicate prefix:
// "isÉtat" is is + titlecase "État" and "isǅefg" is is + a titlecase rune, so the
// first rune (not the lead byte) marks the boundary and neither must be flagged.
func unicodeBoundary(isÉtat, isǅefg bool) {
	_, _ = isÉtat, isǅefg
}

// exportedParam exercises the exported-name guard: an uppercase-first parameter
// is diagnosed but never renamed — the heuristic only preserves unexported names.
func exportedParam(
	Force bool, // want `boolean Force should use an is/has/can/should/will prefix or an Enabled/Disabled suffix`
) bool {
	return Force
}

// collision exercises the sibling-collision guard: found's proposed name isFound
// is already declared in the same signature scope, so no fix is offered.
func collision(
	found bool, // want `boolean found should use an is/has/can/should/will prefix or an Enabled/Disabled suffix`
	isFound bool,
) bool {
	return found && isFound
}

// isCached collides with cached's proposed name from the package scope.
var isCached = true

// packageCollision exercises the enclosing-scope collision guard: cached's
// proposed name isCached is a package-scope declaration, so no fix is offered.
func packageCollision(
	cached bool, // want `boolean cached should use an is/has/can/should/will prefix or an Enabled/Disabled suffix`
) bool {
	return cached && isCached
}

// nestedCollision exercises the nested-scope collision guard where the refusal
// is real: the block declaring isOk also READS the parameter, so the rename
// would put that read under the local declaration and rebind it — source that
// still compiles and means something else. No fix is offered.
func nestedCollision(
	ok bool, // want `boolean ok should use an is/has/can/should/will prefix or an Enabled/Disabled suffix`
) bool {
	if ok {
		isOk := true
		return isOk && ok
	}
	return ok
}

// nestedShadow exercises the same guard in the direction where the shadowing
// captures nothing, which is the direction it used to refuse anyway. The block
// declares isReady and reads nothing of the parameter, so the renamed parameter
// is merely shadowed inside that block — legal Go that moves nothing — and
// withholding the fix there strands the name forever, since no later run finds
// the source any different.
func nestedShadow(
	ready bool, // want `boolean r.ady should use an is/has/can/should/will prefix or an Enabled/Disabled suffix`
) bool {
	if ready {
		isReady := true
		return isReady
	}
	return ready
}

// naked exercises a named result referenced by assignment and a naked return:
// both the declaration and the assignment must be rewritten.
func naked() (done bool) { // want `boolean done should use an is/has/can/should/will prefix or an Enabled/Disabled suffix`
	done = true
	return
}

// literal exercises a func-literal parameter: the declaration and the body
// reference must both be rewritten.
var literal = func(
	valid bool, // want `boolean valid should use an is/has/can/should/will prefix or an Enabled/Disabled suffix`
) bool {
	return valid
}

// Prober exercises an interface-method named result: a bodyless signature name
// has no references, so the rename is a pure declaration edit.
type Prober interface {
	Probe() (ok bool) // want `boolean ok should use an is/has/can/should/will prefix or an Enabled/Disabled suffix`
}

// Hooks exercises a func-type struct field's parameter: the field name itself is
// not boolean, but the signature name inside its type is fixable in place.
type Hooks struct {
	OnChange func(
		flag bool, // want `boolean flag should use an is/has/can/should/will prefix or an Enabled/Disabled suffix`
	)
}

// accent exercises a multi-byte first rune: the whole rune (not the lead byte)
// must be upcased, so état becomes isÉtat.
func accent(
	état bool, // want `boolean état should use an is/has/can/should/will prefix or an Enabled/Disabled suffix`
) bool {
	return état
}

// noop exercises a function with no results.
func noop() {}
