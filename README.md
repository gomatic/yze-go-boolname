# yze-go-boolname

A [`yze`](https://github.com/gomatic/yze) analyzer (group `go`, category `naming`) enforcing the gomatic Go boolean naming standard: boolean fields, parameters, and named results carry an `Is`/`Has`/`Can`/`Should`/`Will` predicate prefix, or an `Enabled`/`Disabled` flag suffix. Underlying types are resolved, so named boolean types (e.g. `uppercaseEnabled`) are checked too.

- **Rule:** `yze/go/boolname`
- **Library:** exports `Analyzer` and `Registration` for the [`yze`](https://github.com/gomatic/yze) aggregator and [`stickler`](https://github.com/gomatic/stickler) runner.
- **Binary:** `cmd/yze-go-boolname` runs it standalone (`text`/`-json`/`-fix`, and as a `go vet -vettool`).

Built on the [`go-yze`](https://github.com/gomatic/go-yze) framework.
