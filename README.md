```
gofmt -w ledger/ledger.go     # fixes spacing: `from,to`, `==0`, `map [Address] uint64` etc.
go build ./...                # should print nothing
go vet ./...                  # should print nothing

```

```
go test ./...                # quick: ok or FAIL
go test -v ./ledger          # verbose: every subtest by name
go test -race -cover ./...   # race detector + % of code the tests exercise
```
