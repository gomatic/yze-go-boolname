# yze-go-boolname

A [`yze`](https://github.com/gomatic/yze) analyzer (category `naming`) enforcing the gomatic Go boolean naming standard: boolean fields, parameters, and named results carry an `is`/`has`/`can`/`should`/`will` predicate prefix (at a word boundary), or an `Enabled`/`Disabled` flag suffix. Underlying types are resolved, so named boolean types (e.g. `type Flag bool`) are checked too.

It offers a **mechanical fix** (`yze --fix`; `gopls` surfaces it as a quick-fix) for parameters and named results only: the rename is `is` + upper-cased first rune (`found` → `isFound`), skipped when the proposed name would collide with any enclosing or nested declaration. The fix rewrites every code reference **and sweeps prose comments**: exact word-boundary mentions of the old name in the enclosing function's doc comment and in comments inside its body (only the literal's own comments for a func-literal parameter) are rewritten too, so docs never go stale — mentions inside longer identifiers (`dryRun`, `laundry`) and other functions' comments are left alone. Struct fields and exported names are never renamed — the production driver loads packages without `_test.go` files, so a field rename could silently break references the pass cannot see.

- **Rule:** `yze/boolname`
- **Library:** exports `Analyzer` and `Registration` for the [`yze`](https://github.com/gomatic/yze) aggregator and [`stickler`](https://github.com/gomatic/stickler) runner.
- **Binary:** `cmd/yze-go-boolname` runs it standalone (`text`/`-json`/`-fix`, and as a `go vet -vettool`).

Built on the [`go-yze`](https://github.com/gomatic/go-yze) framework.
