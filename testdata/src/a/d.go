package a

// Dotted exercises the ASCII-exact prefix match: İ (U+0130) is not the ASCII
// letter i, so İsReady carries no recognized is-prefix and must be flagged as
// unprefixed — non-ASCII predicate spellings are deliberately out of scope.
// The prefix match never slices the name mid-rune, and a struct field gets no
// rename fix.
type Dotted struct {
	İsReady bool // want `boolean İsReady should use an is/has/can/should/will prefix or an Enabled/Disabled suffix`
}

// pointer exercises a *bool parameter: one pointer level is unwrapped, so the
// ill-named pointer parameter is flagged and renamed like a plain bool.
func pointer(
	verbose *bool, // want `boolean verbose should use an is/has/can/should/will prefix or an Enabled/Disabled suffix`
) bool {
	return *verbose
}

// pointerNamed exercises a predicate-named *bool parameter: unwrapped to bool
// and accepted, so it is never flagged.
func pointerNamed(isVerbose *bool) bool {
	return *isVerbose
}

// indirect exercises **bool: only ONE pointer level is unwrapped, so deeper
// indirection is deliberately out of scope and never flagged.
func indirect(chatty **bool) bool {
	return **chatty
}
