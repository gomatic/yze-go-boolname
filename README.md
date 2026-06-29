# yze-boolname

A [`yze`](https://github.com/gomatic/yze) analyzer (category `naming`) enforcing the gomatic Go boolean naming standard: boolean fields, parameters, and named results carry an `is`/`has`/`can`/`should`/`will` predicate prefix (at a word boundary), or an `Enabled`/`Disabled` flag suffix. Underlying types are resolved, so named boolean types (e.g. `type Flag bool`) are checked too.

- **Rule:** `yze/boolname`
- **Library:** exports `Analyzer` and `Registration` for the [`yze`](https://github.com/gomatic/yze) aggregator and [`stickler`](https://github.com/gomatic/stickler) runner.
- **Binary:** `cmd/yze-boolname` runs it standalone (`text`/`-json`, and as a `go vet -vettool`).

Built on the [`go-yze`](https://github.com/gomatic/go-yze) framework.
