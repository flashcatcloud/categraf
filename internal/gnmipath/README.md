# gNMI path parser

`path.go` and `xpath.go` retain the XPath parser from
[`google/gnxi` at `ef18504847b0`](https://github.com/google/gnxi/tree/ef18504847b0/utils/xpath),
the exact revision used before the Docker SDK migration. Changes are limited to
the package name, provenance comments and two equivalent `ReplaceAll` calls to
satisfy the project's lint rules. These files are licensed under Apache-2.0;
the original copyright headers and license are included here.

Categraf only used this parser from `google/gnxi`. That module also required old
Docker/Moby daemon modules for an unrelated orchestrator. Keeping the small pure
parser locally removes that dependency without changing gNMI subscription,
prefix, origin, key escaping or validation behavior. Compatibility tests live in
this package and `inputs/gnmi/path_test.go`.
