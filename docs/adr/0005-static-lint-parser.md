# ADR 0005: Keep the lint declaration parser independent of application code

Status: Accepted when #77 merges.

## Context

Lint analyzes candidate source before maintainers trust it.
The shared Logger imports configuration and mutator packages.
A Go executable that imports Logger also executes initializers in those candidate application packages.
This breaks the static-analysis boundary, even when module downloads are disabled.

## Decision

The declaration parser uses only the Go standard library.
Its architecture model allows no application imports.
Build and execute its explicit `main.go` file with module mode disabled.
Use an empty temporary GOPATH so application and third-party imports cannot resolve.
Do not force the analysis target architecture on the executable parser.
Go selects the host architecture and OS after ambient build overrides are cleared.

Use standard-library logging for this helper's fixed error message.
This is a narrow exception to Logger reuse in ADR 0004.
Keep application diagnostics and other existing tools under their current rules.

## Consequences

Application and dependency initializers cannot run through the declaration parser.
The helper parses source text received through stdin. It never imports scanned source.
Module downloads, custom workspaces, build overrides, and source generators remain disabled.
The Python adapter and parser file remain trusted development tooling that needs policy review when changed.
This boundary does not protect against an arbitrary rewrite of those trusted tools.
CI continues to run candidate tooling without secrets.
