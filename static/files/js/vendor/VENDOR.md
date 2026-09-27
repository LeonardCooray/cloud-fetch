# Vendored browser libraries

Pinned copies, loaded through the import map in `index.html`. To update one: run `npm pack <package>@<version>` in a scratch directory, copy the file listed under Source over the vendored file, then update its version and SHA-256 here. `vendor.test.mjs` fails until the checksum matches.

| File | Package | Version | Source (inside the npm tarball) | SHA-256 |
|---|---|---|---|---|
| `preact.mjs` | preact | 10.29.8 | package/dist/preact.module.js | c30e721ebfdc6e2ad4c18c14d2dfb82667829c8aec27de1207774e3fc16858a8 |
| `hooks.mjs` | preact | 10.29.8 | package/hooks/dist/hooks.module.js | a6ee626f2d01570592dd569a792e3f050154aa02890eead8c223fa3ed5aa3d5a |
| `htm.mjs` | htm | 3.1.1 | package/dist/htm.module.js | ab33dd3f38059b9be4d5f5350128eefb2356639c4e0bbe9d9e8b3ba75847e9e4 |
