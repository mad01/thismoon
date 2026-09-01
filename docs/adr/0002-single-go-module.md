# Single Go module for the whole repo

One `go.mod` at the root (`github.com/mad01/thismoon`, go 1.26.2); components are plain packages inside it. webkit becomes an in-module package, which kills the pseudo-version pin dance where every service repo had to bump its webkit dependency after each change (`update-webkit-all`). The trade-off: a breaking webkit change touches all its consumers in one PR. That is accepted, since the same person maintains both sides and atomic updates are the point.
