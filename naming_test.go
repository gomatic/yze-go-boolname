package boolname

// White-box tests for the naming rules. They are stated in bytes and runes, and
// the analyzer they serve REWRITES source, so a rule that misjudges a boundary
// does not return a wrong answer — it renames the wrong thing, or slices a
// name mid-rune and emits source that will not parse.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHasASCIIPrefixFoldMatchesExactlyTheASCIICaseFold names the claim that
// `name[i]|0x20 != prefix[i]` folds case for ASCII and NOTHING else. Setting
// the 0x20 bit is a case fold only inside the ASCII letter range; the
// recognised prefixes are pure lowercase ASCII, so the test is which bytes can
// possibly match one.
//
// The failure this forbids is a false match on a multi-byte rune: if any byte
// of a UTF-8 sequence could fold onto a prefix byte, the analyzer would slice
// name[len(prefix):] mid-rune and propose a rename that is not valid UTF-8.
func TestHasASCIIPrefixFoldMatchesExactlyTheASCIICaseFold(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   identName
		prefix predicatePrefix
		why    string
		want   bool
	}{
		{name: "isReady", prefix: "is", want: true, why: "exact lowercase"},
		{name: "ISReady", prefix: "is", want: true, why: "uppercase folds"},
		{name: "IsReady", prefix: "is", want: true, why: "mixed case folds"},
		{name: "us", prefix: "is", want: false, why: "a different letter is not a fold"},
		{name: "i", prefix: "is", want: false, why: "shorter than the prefix"},
		{name: "İsReady", prefix: "is", want: false, why: "U+0130 is multi-byte and must never fold onto ASCII"},
		{name: "\xc4\xb0s", prefix: "is", want: false, why: "no UTF-8 continuation byte may fold onto a prefix byte"},
		{name: "IS", prefix: "is", want: true, why: "both bytes fold"},
	} {
		assert.Equal(t, tc.want, hasASCIIPrefixFold(tc.name, tc.prefix),
			"hasASCIIPrefixFold(%q, %q): %s", tc.name, tc.prefix, tc.why)
	}
}

// TestHasASCIIPrefixFoldRejectsEveryNonASCIILeadingByte is the exhaustive form
// of the claim above: no byte with the high bit set may ever match a prefix
// byte, because such a byte is only ever part of a multi-byte rune.
func TestHasASCIIPrefixFoldRejectsEveryNonASCIILeadingByte(t *testing.T) {
	t.Parallel()

	for _, prefix := range prefixes {
		for b := 0x80; b <= 0xFF; b++ {
			name := identName(append([]byte{byte(b)}, prefix[1:]...))
			assert.False(t, hasASCIIPrefixFold(name, prefix),
				"byte 0x%02X must not fold onto prefix %q", b, prefix)
		}
	}
}

// TestMatchesPrefixRequiresAWordBoundaryAfterThePrefix names matchesPrefix'
// claim. "island" begins with "is" as bytes but the name is not a predicate —
// renaming it would be a false positive that rewrites correct code. The
// boundary is an uppercase rune, which is also what guarantees the slice at
// len(prefix) lands on a rune boundary.
func TestMatchesPrefixRequiresAWordBoundaryAfterThePrefix(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name identName
		why  string
		want bool
	}{
		{name: "isReady", want: true, why: "prefix then an uppercase word"},
		{name: "island", want: false, why: "prefix continues into another word"},
		{name: "is", want: false, why: "prefix with nothing after it"},
		{name: "is_ready", want: false, why: "the rest must start uppercase"},
		{name: "isÉtat", want: true, why: "a non-ASCII uppercase rune is still a boundary"},
		{name: "isétat", want: false, why: "and its lowercase is still not one"},
	} {
		assert.Equal(t, tc.want, matchesPrefix(tc.name, "is"),
			"matchesPrefix(%q, \"is\"): %s", tc.name, tc.why)
	}
}

// TestReferencesToFindsTheDeclarationAndEveryRead names referencesTo's claim
// that it is EXACTLY the set a rename rewrites. Both halves are load-bearing
// and each is missed by a different failure. Miss the declaration and the
// rewrite leaves it behind, so every read now names an undefined symbol; miss a
// read and the read keeps the old spelling, which is the same error the other
// way round. And the set is keyed on the OBJECT, never on the spelling: another
// symbol with the same name in another signature is a different object and must
// not be swept in, or the rename reaches into a function it was never about.
func TestReferencesToFindsTheDeclarationAndEveryRead(t *testing.T) {
	t.Parallel()
	src := checked(t, `package p
func subject(ready bool) bool {
	if ready {
		return ready
	}
	return !ready
}
func elsewhere(ready bool) bool { return ready }
`)
	name := paramOf(t, src.file, "subject")
	found := referencesTo(src.pass, src.pass.TypesInfo.Defs[name])

	require.Len(t, found, 4, "the declaration and its three reads, and nothing from elsewhere")
	assert.Same(t, name, found[0], "the declaration comes first, in source order")
	for _, id := range found[1:] {
		assert.Equal(t, "ready", id.Name)
		assert.Less(t, id.Pos(), paramOf(t, src.file, "elsewhere").Pos(),
			"every reference is inside subject; elsewhere declares a different object of the same name")
	}
}
