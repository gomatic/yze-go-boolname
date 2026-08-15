package a

// proposalCollision exercises the guard on the analyzer's OWN proposals: the
// ASCII s and the long s ſ (U+017F) upper-case to the same S, so both
// parameters propose the same name. Neither name exists yet, so the scope
// lookup sees no collision; the first claimant keeps it and the second is
// diagnosed without a fix, because a signature declaring one identifier twice
// does not compile.
func proposalCollision(
	slow bool, // want `boolean slow should use an is/has/can/should/will prefix or an Enabled/Disabled suffix`
	ſlow bool, // want `boolean ſlow should use an is/has/can/should/will prefix or an Enabled/Disabled suffix`
) bool {
	return slow && ſlow
}

// resultCollision exercises the same guard across the parameter/result
// boundary: a signature's parameters and its named results share one scope, so
// a result contends with a parameter for a proposed name. Here the ASCII i and
// the dotless ı (U+0131) upper-case to the same I.
func resultCollision(
	idle bool, // want `boolean idle should use an is/has/can/should/will prefix or an Enabled/Disabled suffix`
) (ıdle bool) { // want `boolean ıdle should use an is/has/can/should/will prefix or an Enabled/Disabled suffix`
	ıdle = idle
	return
}

// underscoreLed exercises the guard on a proposal the rule itself rejects: an
// underscore has no upper case, so the prefix would gain no word boundary and
// the proposed name would be diagnosed the moment it was written — growing
// another "is" on every subsequent run. No fix is offered.
func underscoreLed(
	_chatty bool, // want `boolean _chatty should use an is/has/can/should/will prefix or an Enabled/Disabled suffix`
) bool {
	return _chatty
}

// caselessScript exercises the same guard for a caseless script: Han has no
// case at all, so no is-prefixed rename of such a name can ever satisfy the
// rule and none is proposed.
func caselessScript(
	有効 bool, // want `boolean 有効 should use an is/has/can/should/will prefix or an Enabled/Disabled suffix`
) bool {
	return 有効
}
