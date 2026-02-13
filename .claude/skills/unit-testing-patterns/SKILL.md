---
name: unit-testing-patterns
description: Unit testing patterns for this project
---

When writing unit tests for this project, ensure to always follow the following patterns:
* Always include a test function for each exported function or method
* Test error handling paths
* Test edge cases and boundary conditions
* Use mock objects for dependencies
* Test for expected behavior and not implementation details
* Use assertions to verify expected results
* Use subtests to group related tests
* Use a separate test file for each package
* mocks should be shared between tests, under `tests/mocks`
* NEVER call external services in tests
* ALWAYS mock external services in tests
* **ALWAYS use test utilities from `test/` package** when writing unit tests
* Use `test.CreateTestLogger(t)` to create loggers in tests (integrates with Go testing framework, follows zap best practices)
* Never use `zap.NewNop()` or create loggers manually - always use test utilities
* Write table-driven tests with clear input/output expectations
* Use package `_test` suffix (e.g., `configloader_test`) for external testing perspective 
* Include detailed error messages (expected vs. actual)
* Test every exported function and error case 
* For external test packages, explicitly declare types when using imported packages to avoid "imported and not used" errors
