---
name: go-mockery
description: Blueprint and good practices to implement unit tests using Mockery Golang Library.
---

# go-mockery

1. Install mockery dependency if not exists: `go get github.com/vektra/mockery`.
2. Install testify if not already installed: `github.com/stretchr/testify`.

## When to use

I you need implement tests in golang with mocking features.

## Instructions

1. Create mocks of required interfaces using a mockery configuration file and the command `mockery`. See the example for more information: `references/examples/.mockery.yaml`.
2. Create tests using mocks and testify. See the example for more information: `references/examples/main_test.go`.
3. Run tests to check if all it's working good.

## References

See more information here

- Mockery configuration example in `references/examples/.mockery.yaml`.
- Test example using mockery generated code in `references/examples/main_test.go`.
- Mockery official documentation in markdown:
  - https://raw.githubusercontent.com/vektra/mockery/refs/heads/v3/docs/configuration.md
