# Changelog

## [0.4.0](https://github.com/supabase-community/scim-go/compare/v0.3.0...v0.4.0) (2026-09-10)


### Features

* add API to compile filter into datastore optimized query language like SQL ([11404e6](https://github.com/supabase-community/scim-go/commit/11404e693b82cf7a4afdc440718a8e5c268854d2))
* **core:** add attribute builders and binary type ([b3322ba](https://github.com/supabase-community/scim-go/commit/b3322ba31aadc6306855f9121e26de2d240cc114))
* **core:** validate base64 for binary attribute values ([5fbc789](https://github.com/supabase-community/scim-go/commit/5fbc789d5242ac5e9bffca08ce7f54e78997c5c5))
* **filter:** set max filter input size to 8K bytes ([feba5d8](https://github.com/supabase-community/scim-go/commit/feba5d89d3942535b9d45fef3d39fc794aa9bc37))


### Bug Fixes

* **filter:** preserve numeric literals as json.Number to keep integer precision ([04914b9](https://github.com/supabase-community/scim-go/commit/04914b9ccca65c80f99aea792601e03653c13399))
* **protocol:** use invalidFilter scimType for filter attribute errors ([f7657d7](https://github.com/supabase-community/scim-go/commit/f7657d75c2b04ec4ea05542d6bbe4242faf28f84))

## [0.3.0](https://github.com/supabase-community/scim-go/compare/v0.2.0...v0.3.0) (2026-09-03)


### Features

* add SCIM filter grammar and parser ([ddcc45d](https://github.com/supabase-community/scim-go/commit/ddcc45dab632739fd45e7d88e88d56da00a7ebb7))

## [0.2.0](https://github.com/supabase-community/scim-go/compare/v0.1.0...v0.2.0) (2026-09-03)


### Features

* use stdlib uuid package ([bcbede9](https://github.com/supabase-community/scim-go/commit/bcbede951f1a3ea3bbcdfbe83438a10257df6f58))
