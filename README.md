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
## Show only the failures (filters out all the PASS lines):
```
go test ./... 2>&1 | grep -E "FAIL|_test.go"

```
## Run just the failing test, much less noise:
```
go test -v -run TestMemTransportFIFO ./p2p

```

## GoRoutines & Race-Condition and Mutex
1. Without the race detector:
   ```
   go test -count=1 -v -run Concurrent ./p2p

   ```
2. With the race detector:
   ```
   go test -count=1 -race -run Concurrent ./p2p

   ```
3. Run everything with the race detector after using mutex
```
gofmt -w . && go vet ./... && go test -count=3 -race -v -cover ./...

```
