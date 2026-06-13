## 1. CLI discovery

- [x] 1.1 Add failing CLI test for hidden root `pinax schema export` compatibility path and root help hiding.
- [x] 1.2 Implement shared schema command builder for `pinax api schema` and hidden root `pinax schema`.

## 2. API routes human output

- [x] 2.1 Add failing CLI test requiring default `pinax api routes` output to show REST/RPC endpoint evidence.
- [x] 2.2 Add route evidence and schema export next action to the shared `api.routes` projection.

## 3. Verification

- [x] 3.1 Ran `go test ./cmd/pinax -run 'TestCLITree(HelpSmoke|PrimaryPathAliases)' -count=1` and observed the intended RED failure for missing root `schema` before implementation.
- [x] 3.2 Ran `go test ./cmd/pinax -run TestAPIRoutesHumanOutputListsEndpointsCLI -count=1` and observed the intended RED failure for missing route endpoint evidence before implementation.
- [x] 3.3 Ran `go test ./cmd/pinax -run 'Test(APIRoutesHumanOutputListsEndpointsCLI|CLITree(HelpSmoke|PrimaryPathAliases))' -count=1` after implementation; passed.
- [x] 3.4 Ran `task check`; passed with `openspec validate --all`, `golangci-lint run`, `golangci-lint fmt --diff`, `go test ./...`, and `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`.
