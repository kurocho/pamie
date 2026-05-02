## Summary

Describe the change and the user-facing behavior it affects.

## Validation

- [ ] `go fmt` or `make fmt`
- [ ] `go test ./...`
- [ ] `go vet ./...`
- [ ] `go build ./cmd/pamie`

## Security

- [ ] No raw SQL tool surface was added.
- [ ] No shell execution tool surface was added.
- [ ] Stored memory content is treated as untrusted input.
- [ ] Public endpoints remain protected or explicitly documented as unauthenticated.

## Documentation

- [ ] Architecture, roadmap, tasks, or security docs were updated when behavior changed.
