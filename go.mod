module github.com/dconsole/dconsole-acquia

go 1.24

// dconsole provides the plugin wire-protocol types we implement.
require github.com/dconsole/dconsole v0.5.14

// During local co-development, point at the sibling dconsole checkout
// so changes to pkg/plugin flow through without a release cycle.
// Comment this out before tagging a release so the pinned module
// version above is what's built.
replace github.com/dconsole/dconsole => ../dconsole
