Shared protocol contract

Purpose:
- Keep backend implementations (Go and C++) interoperable.
- Keep Java frontend integration stable regardless of backend language choice.

Current plan:
- Define peer discovery messages and sync metadata once.
- Generate language bindings from this contract where useful.

