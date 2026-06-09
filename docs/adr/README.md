# Architecture Decision Records

This directory holds Architecture Decision Records (ADRs): one Markdown file per
decision, numbered in order (`NNNN-title.md`).

- [0001](0001-go-crypto-fork-vs-upstream.md) - `ProtonMail/go-crypto` `v1.4.1-proton` → `v1.4.1` swap is safe
- [0002](0002-remote-transport-sse-vs-streamable-http.md) - Keep legacy SSE for the remote transport; defer Streamable HTTP
- [0003](0003-auto-reauth-on-scope-insufficiency.md) - Self-heal under-scoped sessions by reusing the unattended relogin (Proposed)
