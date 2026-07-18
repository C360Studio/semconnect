# Go developer verification

Final verification was rerun after all task 2 source/pin edits and final `go mod tidy` normalization.

## Unit tests

```console
$ go test ./...
?       github.com/c360studio/semconnect/cmd/cs-api-server [no test files]
ok      github.com/c360studio/semconnect/conformance  0.170s
ok      github.com/c360studio/semconnect/conformance/cmd/index-readiness  (cached)
ok      github.com/c360studio/semconnect/gateway/cs-api  0.480s
ok      github.com/c360studio/semconnect/message/oms  (cached)
ok      github.com/c360studio/semconnect/parser/sensorml  (cached)
ok      github.com/c360studio/semconnect/pkg/swecommon  (cached)
ok      github.com/c360studio/semconnect/vocabulary/csapi  (cached)
ok      github.com/c360studio/semconnect/vocabulary/oms  (cached)
ok      github.com/c360studio/semconnect/vocabulary/sosa  (cached)
ok      github.com/c360studio/semconnect/vocabulary/swe  (cached)
```

Exit code: `0`.

## Race tests

```console
$ go test -race ./...
?       github.com/c360studio/semconnect/cmd/cs-api-server [no test files]
ok      github.com/c360studio/semconnect/conformance  1.243s
ok      github.com/c360studio/semconnect/conformance/cmd/index-readiness  (cached)
ok      github.com/c360studio/semconnect/gateway/cs-api  1.968s
ok      github.com/c360studio/semconnect/message/oms  (cached)
ok      github.com/c360studio/semconnect/parser/sensorml  (cached)
ok      github.com/c360studio/semconnect/pkg/swecommon  (cached)
ok      github.com/c360studio/semconnect/vocabulary/csapi  (cached)
ok      github.com/c360studio/semconnect/vocabulary/oms  (cached)
ok      github.com/c360studio/semconnect/vocabulary/sosa  (cached)
ok      github.com/c360studio/semconnect/vocabulary/swe  (cached)
```

Exit code: `0`.

## Real-NATS integration tests

```console
$ go test -tags=integration ./...
?       github.com/c360studio/semconnect/cmd/cs-api-server [no test files]
ok      github.com/c360studio/semconnect/conformance  0.206s
ok      github.com/c360studio/semconnect/conformance/cmd/index-readiness  (cached)
ok      github.com/c360studio/semconnect/gateway/cs-api  0.513s
ok      github.com/c360studio/semconnect/message/oms  (cached)
ok      github.com/c360studio/semconnect/parser/sensorml  (cached)
ok      github.com/c360studio/semconnect/pkg/swecommon  (cached)
ok      github.com/c360studio/semconnect/vocabulary/csapi  (cached)
ok      github.com/c360studio/semconnect/vocabulary/oms  (cached)
ok      github.com/c360studio/semconnect/vocabulary/sosa  (cached)
ok      github.com/c360studio/semconnect/vocabulary/swe  (cached)
```

Exit code: `0`.

## Static analysis and build

```console
$ go vet ./...
$ go vet -tags=integration ./...
$ go build ./...
```

Each command produced no output and exited `0`.
