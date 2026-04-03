# Context Standards

When working with golang's `context.Context` type, there are a few code standards that should be followed.
* Always check for cancellation, especially before doing expensive work.
* Always pass context as the first parameter to a function.
* NEVER store context values in a struct.
* NEVER use the context's Done channel directly.
* NEVER pass nil context.
* context.WithValue should only be used for carrying request-scoped, immutable data across API boundaries and between processes (e.g., security credentials, tracing IDs, or an authenticated user's details).
* To avoid key collisions when using WithValue, define a custom, unexported type for your context keys, often an empty struct, rather than using basic types like string or int.
* Do not use Context to pass optional function parameters; use regular function arguments for operational data.
* Avoid storing large data in the context. Use dedicated function parameters or structs for large data payload