package a

// rinse reports whether dry mode was requested: this doc mention of dry is in
// the renamed symbol's scope and must be rewritten, while dryRun, dry_run,
// dry2, and laundry have no word boundary and must all stay untouched.
func rinse(
	dry bool, // want `boolean dry should use an is/has/can/should/will prefix or an Enabled/Disabled suffix`
) bool {
	// dry gates the early return; dryRun and laundry must stay.
	if dry {
		return true
	}
	/* dry falls through past the block comment too */
	return dry
}

// spin also says dry in its doc, but it is another function: renaming rinse's
// dry parameter must leave this comment and the one in spin's body untouched.
func spin(isFast bool) bool {
	// dry belongs to rinse's neighbor here, so it must stay untouched.
	return isFast
}

// wrap says wet right here in its doc, but the renamed wet is the literal's
// parameter: only comments inside the literal are swept, so this doc stays.
func wrap() func(bool) bool {
	return func(
		wet bool, // want `boolean wet should use an is/has/can/should/will prefix or an Enabled/Disabled suffix`
	) bool {
		// wet is the literal's own comment and must be rewritten.
		return wet
	}
}

// stub is a bodyless declaration (implemented outside Go): raw appears only in
// this doc, which is swept even though there is no body to sweep.
func stub(
	raw bool, // want `boolean raw should use an is/has/can/should/will prefix or an Enabled/Disabled suffix`
) bool
