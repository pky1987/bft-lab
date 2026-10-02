## go Build & format :

```
gofmt -w ledger/ledger.go     # fixes spacing: `from,to`, `==0`, `map [Address] uint64` etc.
go build ./...                # should print nothing
go vet ./...                  # should print nothing

```

## go test run command:
```
go test ./...                # quick: ok or FAIL
go test -v ./ledger          # verbose: every subtest by name
go test -race -cover ./...   # race detector + % of code the tests exercise
```
## go fuzz test command: 
```
go test -v -run FuzzTransfer ./ledger                      # 1. seeds only, fast, runs in normal CI
go test -fuzz=FuzzTransfer -fuzztime=30s ./ledger          # 2. real fuzzing for 30 seconds

```
## Run all at once:

```
gofmt -w . && go vet ./... && go test -race -v -cover ./...

```
