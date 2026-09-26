# Changelog

## [0.6.0](https://github.com/supabase-community/scim-go/compare/v0.5.0...v0.6.0) (2026-09-26)


### ⚠ BREAKING CHANGES

* **core:** move basePath from core to server
* **server:** build the in-memory repository from explicit arguments
* **server:** customize ServiceProviderConfig and unexport internal helpers
* **server:** configure limits and error handler as options on New
* **server:** require WithRepository and add NewMemoryRepository
* **server:** validators take Fields and Uniqueness queries only matching resources
* **core:** let User carry password through JSON so PATCH keeps it
* **server:** add schema extensions to resource types
* **server:** copy resources in and out of the in-memory repository under a lock
* model multi-valued elements and evaluate value paths per element
* **protocol:** pass *protocol.Attribute to Evaluator instead of a key string, extract visitor into visitor.go
* **core:** drop description arg from NewAttribute, add DescribedAs

### Features

* **core:** add Authentication builder to ServiceProviderConfig ([62baec5](https://github.com/supabase-community/scim-go/commit/62baec5594a397664a528304e4ce3562fdb6cea8))
* **core:** add Base accessor methods ([14def7e](https://github.com/supabase-community/scim-go/commit/14def7e88f99be7ab7001207805ff500b8ba2df2))
* **core:** add CommonAttribute lookup helper ([e4bc947](https://github.com/supabase-community/scim-go/commit/e4bc947dbf361933d0f6a0c0a2ae6f83aa19c607))
* **core:** add EnterpriseUserAttributes for the RFC 7643 enterprise User extension ([dd7fa20](https://github.com/supabase-community/scim-go/commit/dd7fa20bf971ce427148f5b9a17d7805ae3c1734))
* **core:** add GroupAttributes from RFC 7643 Section 4.2 ([649e009](https://github.com/supabase-community/scim-go/commit/649e0098626ae53e919d9a9fe2158742d7f94bdc))
* **core:** add method to retrive external id ([6aae0df](https://github.com/supabase-community/scim-go/commit/6aae0dfec1445c922c6e9f9eb28e4883e2748e6e))
* **core:** add ServiceProviderConfig constructor and Versioning builder ([2619a76](https://github.com/supabase-community/scim-go/commit/2619a7653ee9626edbbce402078b4015e2d183eb))
* **core:** add UserAttributes from RFC 7643 Section 4.1 ([d2b8924](https://github.com/supabase-community/scim-go/commit/d2b89248e672bdde03ad9b4552447ee7a1640662))
* **patch:** refuse PATCH requests whose value filters check more than 10 million clauses ([e1648b2](https://github.com/supabase-community/scim-go/commit/e1648b2e5a6f01a65266cf90daaed1a84f580426))
* **protocol:** add Attribute type wrapping core.Attribute with AttrPath ([46bb0d5](https://github.com/supabase-community/scim-go/commit/46bb0d5f56a7588896735459f2d27dcacc8934fb))
* **protocol:** project resources by attributes, excludedAttributes and returned ([6493fe4](https://github.com/supabase-community/scim-go/commit/6493fe41e21c29b01fab90663aad7b5a68254c54))
* **scimerrors:** add ErrPreconditionFailed for If-Match support ([20a441c](https://github.com/supabase-community/scim-go/commit/20a441cd2f6192135a26f44fdd408a7f2d88d3b8))
* **server:** add reference implementation server ([d7a3fbb](https://github.com/supabase-community/scim-go/commit/d7a3fbbf52088fabf5e751961fcfe239770a14ea))
* **server:** add schema extensions to resource types ([94bfc4b](https://github.com/supabase-community/scim-go/commit/94bfc4bb5e0577f6973f2cc96c8b0b4f8cea9d29))
* **server:** add the enterprise User extension to the example server ([3cd53a6](https://github.com/supabase-community/scim-go/commit/3cd53a626da79f37b91915d39d2bb48270a5568b))
* **server:** apply attributes, excludedAttributes and returned to every returned resource ([758f4d2](https://github.com/supabase-community/scim-go/commit/758f4d2b5a7b8ff4a87629d1ebcd112a1be861e4))
* **server:** cap PATCH requests at 100 operations by default with 413 beyond it ([0d6e18f](https://github.com/supabase-community/scim-go/commit/0d6e18ffbfaaa797cfdc19d4837048c9257fb7bd))
* **server:** configure limits and error handler as options on New ([197b654](https://github.com/supabase-community/scim-go/commit/197b654ebc7cd9e4cc38178ca3e683d50cca3746))
* **server:** customize ServiceProviderConfig and unexport internal helpers ([b507b9c](https://github.com/supabase-community/scim-go/commit/b507b9c09db1c730caf538e6affa89be357bdd8e))
* **server:** default resources to an in-memory repository ([baac672](https://github.com/supabase-community/scim-go/commit/baac6722b55ead64004f9f2af9e93805a7ae27ec))
* **server:** drive routes and behavior from ServiceProviderConfig ([7421d89](https://github.com/supabase-community/scim-go/commit/7421d89756c1010b1a15d169e8ecd7fca3dd543f))
* **server:** include the common accessors ([f862579](https://github.com/supabase-community/scim-go/commit/f862579c6c5ee5c898a664fd69ef1e7709ac2d95))
* **server:** pass the request to ErrorHandler ([a303208](https://github.com/supabase-community/scim-go/commit/a303208562e49968e9f577b52a9a8fbae11ce281))
* **server:** register Group alongside User in cmd/server ([3e493e3](https://github.com/supabase-community/scim-go/commit/3e493e3377ef7c6a66bed98afb08400388c6771b))
* **server:** require WithRepository and add NewMemoryRepository ([6497e99](https://github.com/supabase-community/scim-go/commit/6497e99528045b0dc611eb26d15860e9e7d732af))
* **server:** set Content-Location to meta.location ([818274c](https://github.com/supabase-community/scim-go/commit/818274c4ba552bbd12845423730b6f0e64de7c79))
* **server:** sort multi-valued attributes by primary or first value ([553e13c](https://github.com/supabase-community/scim-go/commit/553e13cd9d93bf0e65c7e40ae5104341d3daa763))
* **server:** validators take Fields and Uniqueness queries only matching resources ([7923d1e](https://github.com/supabase-community/scim-go/commit/7923d1e28f52205e7f30c0a8487bbad490ed5a39))


### Bug Fixes

* check immutable attributes in the service only so members with immutable sub-attributes can be added and removed ([44b91eb](https://github.com/supabase-community/scim-go/commit/44b91ebe9827e8d69ab327a2e83cfe91676f3276))
* **core:** let User carry password through JSON so PATCH keeps it ([a7cefe0](https://github.com/supabase-community/scim-go/commit/a7cefe096d7ef3f3c92bdf7e34d0b0f6b3d3482a))
* **core:** resolve schema URNs case-insensitively and route PATCH extension paths into the extension object ([39952e8](https://github.com/supabase-community/scim-go/commit/39952e85203ef96ea77b11928670c2184337a6a0))
* **filter:** require SP between filter tokens and reject leading, trailing and non-SP whitespace ([578022c](https://github.com/supabase-community/scim-go/commit/578022cf57d48688d0f2c9588d4182786cbc30f3))
* **patch:** allow only eq on writeOnly or returned never attributes in value filters ([0a697e2](https://github.com/supabase-community/scim-go/commit/0a697e2da395ab80596ab24127bfafbcb27341b6))
* **patch:** cap a value-filtered write's output size to stop holder-count amplification ([7d9ce4c](https://github.com/supabase-community/scim-go/commit/7d9ce4cdbc93d69f663987530f93796b25dbf601))
* **patch:** dedupe PATCH add values and skip no-op persists ([f7309f6](https://github.com/supabase-community/scim-go/commit/f7309f6f6d246ec2f3c3e0091ae9cb01725cb7c4))
* **patch:** merge complex sub-attributes, reject readOnly targets, and require PatchOp schema and operations ([2e87497](https://github.com/supabase-community/scim-go/commit/2e87497eda3f76ca8c20589adb586f54e686d102))
* **patch:** reject a replace that unassigns a readOnly sub-attribute ([470f8d1](https://github.com/supabase-community/scim-go/commit/470f8d1c60b1431023be23a3c6e14fe3c195c578))
* **patch:** reject writes and removes of readOnly sub-attributes in every value shape ([a3de70c](https://github.com/supabase-community/scim-go/commit/a3de70ca26ec6a8d5dfeec63e6ee5510889ee853))
* **patch:** reset primary only on the attribute a patch makes primary ([977ca9a](https://github.com/supabase-community/scim-go/commit/977ca9a1b1c69e107d75c08d414a361ad485478d))
* **patch:** set primary to false on the other values when a patch makes a value primary ([18c5afb](https://github.com/supabase-community/scim-go/commit/18c5afb1d199c949312698a34165dd5dcdafe5e5))
* **protocol:** drop unknown and duplicate names from attributes and excludedAttributes ([931d29e](https://github.com/supabase-community/scim-go/commit/931d29ef2ea5c97425b57143c623bf1432c10e92))
* **protocol:** fill schemas and meta.resourceType when a repository leaves them empty ([f509ca2](https://github.com/supabase-community/scim-go/commit/f509ca2b71406f9379447f123a65d220fd39efb5))
* **protocol:** keep readOnly sub-attributes of elements a PATCH does not target ([6512575](https://github.com/supabase-community/scim-go/commit/6512575bd8b2418bd3d001909f663f2e937e2642))
* **protocol:** keep readOnly sub-attributes of the stored element a PUT element matches ([32dfc0b](https://github.com/supabase-community/scim-go/commit/32dfc0ba71ca333025ec1b99c5ff97f87da37915))
* **protocol:** limit filter, sort, and output of writeOnly and returned never attributes ([a0d9888](https://github.com/supabase-community/scim-go/commit/a0d988827eaf772491ff21a462ec59175407c04e))
* **protocol:** propagate SearchRequest.Projection errors instead of panicking ([9a9997e](https://github.com/supabase-community/scim-go/commit/9a9997e78d6c8adc2c962f50fabd29be68b21b1a))
* **protocol:** refuse GET filters on writeOnly or returned never attributes with 403 sensitive ([33d012d](https://github.com/supabase-community/scim-go/commit/33d012dfeeed3786d9250c740204355dec111502))
* **protocol:** refuse only GET filters that send a hidden attribute value with 403 sensitive ([be3cd72](https://github.com/supabase-community/scim-go/commit/be3cd724fbeb9ae59c6df3609cdf9766930cecf1))
* **protocol:** reject request bodies that repeat an attribute name in another case ([d2566b7](https://github.com/supabase-community/scim-go/commit/d2566b77054e7db503ca50db72a5c5e32538bfc2))
* **protocol:** reject request bodies with attribute names that are not US-ASCII ([f309476](https://github.com/supabase-community/scim-go/commit/f309476da9f1ecdd46c1b2428b04f65e8b1316f0))
* **protocol:** report the cause of a non-SCIM error to ErrorHandler ([2cf7906](https://github.com/supabase-community/scim-go/commit/2cf79064d0bda914a60cbf586c7080381aee0214))
* **protocol:** resolve bare multi-valued attribute names to their value sub-attribute ([0d66ad0](https://github.com/supabase-community/scim-go/commit/0d66ad028d66c62cfd4e0e42ec554afd0b7a4e63))
* **protocol:** resolve filter attributes before the repository sees the query ([2170241](https://github.com/supabase-community/scim-go/commit/217024165b475ec13be073b7832a68f8c3862146))
* **protocol:** send an internal error instead of an empty 200 on encode failure ([dc52618](https://github.com/supabase-community/scim-go/commit/dc52618b2fe12cf485b7a21e75052099469f12f7))
* **protocol:** treat sub-attributes of writeOnly or returned never attributes as hidden in filters and sortBy ([1771957](https://github.com/supabase-community/scim-go/commit/17719579f54713f7114e6017560148562854ee45))
* **server:** answer a patch that loses a race with 409 when the client sent no If-Match ([b8f0ebb](https://github.com/supabase-community/scim-go/commit/b8f0ebb9c4fb66544dfbc74c4ac9cfa1d4152389))
* **server:** answer unknown endpoints and unsupported methods with a SCIM error body ([bd0fcec](https://github.com/supabase-community/scim-go/commit/bd0fcecf2daf3be58c93acd0e583f217f185ebda))
* **server:** answer validator failures with 500 and report them, keep 401 for ErrInvalidToken ([bb70e3a](https://github.com/supabase-community/scim-go/commit/bb70e3ad4cfa5c54093e81636771a51adb649ff0))
* **server:** cap request bodies with 413, validate filter and sortBy before the repository, honour If-Match *, apply options regardless of order, and normalize accessor values ([8d0c55a](https://github.com/supabase-community/scim-go/commit/8d0c55a458dea078114c591f9b1eabc53b5af4be))
* **server:** check attributes in schema order so errors are deterministic ([508558f](https://github.com/supabase-community/scim-go/commit/508558f145890c3d4698d933e1c83601a989989b))
* **server:** check canonical values on each element of a multi-valued attribute ([3a7c476](https://github.com/supabase-community/scim-go/commit/3a7c476cc6be5424f6e7e2ecb3e7c68d8a871f3d))
* **server:** compare-and-swap PATCH on the version read and retry lost races when If-Match is absent ([480a779](https://github.com/supabase-community/scim-go/commit/480a77924bc9c09538416585d7b950c7adbd654a))
* **server:** conform bearer-token challenges to RFC 6750 S3.1 and fail fast on an unset token ([6041496](https://github.com/supabase-community/scim-go/commit/6041496fc3b7ecd643619144ecc6cf85262bcfa9))
* **server:** copy resources in and out of the in-memory repository under a lock ([460ecf2](https://github.com/supabase-community/scim-go/commit/460ecf21c98e0f708a6b13a3e4db475ec9e46f6c))
* **server:** decline the /Me alias with 501 Not Implemented ([b234dee](https://github.com/supabase-community/scim-go/commit/b234dee3d2fb413a407fca2b0f5e91414694bdf0))
* **server:** enforce immutable sub-attributes of multi-valued attributes ([46ca97a](https://github.com/supabase-community/scim-go/commit/46ca97a6f8832a1fe72b4edf783ed76908652638))
* **server:** enforce uniqueness atomically in the repository ([a5531b6](https://github.com/supabase-community/scim-go/commit/a5531b69b50add6c53190cf316cdc3282a1d113f))
* **server:** honor caseExact when comparing immutable values ([a9eb34f](https://github.com/supabase-community/scim-go/commit/a9eb34f001005a98f833c4b0f8acc40f792abf79))
* **server:** ignore readOnly attributes in POST and PUT bodies ([6451756](https://github.com/supabase-community/scim-go/commit/6451756d2100328a6583638d695c6486e495a7b6))
* **server:** include a realm auth-param in every Bearer challenge ([e93387b](https://github.com/supabase-community/scim-go/commit/e93387baf66d53aee8531d8ebe95f5e5297da697))
* **server:** keep a ServiceProviderConfig location set by the caller ([6f03f48](https://github.com/supabase-community/scim-go/commit/6f03f48072f7de71b4d4526221d9beaae95dea05))
* **server:** keep schema extension URIs and make them filterable and sortable ([e8d1d27](https://github.com/supabase-community/scim-go/commit/e8d1d27f862cd832af2c0aac1fb799e250e62dfa))
* **server:** pin version-less PUTs to the read version to close an immutability check race ([46dc7ae](https://github.com/supabase-community/scim-go/commit/46dc7ae82d438181a6abc300dffb9f6747bf25db))
* **server:** reject a Bearer header without a token with 400 invalid_request ([e65a46c](https://github.com/supabase-community/scim-go/commit/e65a46cb9e982ce1f5b83b54675d12861764f98b))
* **server:** reject filter with 403 on ResourceTypes and Schemas ([51611b2](https://github.com/supabase-community/scim-go/commit/51611b2b8b11872aa3f9d0e62d78fc6f842b5c67))
* **server:** reject more than one primary value in a multi-valued attribute ([2dcb632](https://github.com/supabase-community/scim-go/commit/2dcb6322de4e4c55850526a93247ddc421d022a7))
* **server:** require sub-attributes on each element of a multi-valued attribute ([ace2176](https://github.com/supabase-community/scim-go/commit/ace2176e47a5a82e8446d479e92b5179da015919))
* **server:** reset the version before replacing a patched resource ([9180c7e](https://github.com/supabase-community/scim-go/commit/9180c7e781f6d3d8c199557d4ac807d6876cf9f5))
* **server:** restore exported Service and Controller interfaces ([3b3db71](https://github.com/supabase-community/scim-go/commit/3b3db71787616a52ec8ac0c45eeaf6bfa72f2d46))
* **server:** return 501 for /Bulk instead of 404 ([3ec4418](https://github.com/supabase-community/scim-go/commit/3ec4418f310e622bb8abe7cf831c8070ae67e2fe))
* **server:** return 501 for POST .search instead of 404/405 ([ce4ce5a](https://github.com/supabase-community/scim-go/commit/ce4ce5aab26fb6b3964a58569cd10f1a6ed4b1fa))
* **server:** sort missing values first when descending ([c63dcf2](https://github.com/supabase-community/scim-go/commit/c63dcf25726673b5e5a1022a65a9895869280a10))
* **server:** stamp schemas from populated extensions instead of the static registry ([483b573](https://github.com/supabase-community/scim-go/commit/483b5730c9d33828eebe902be93732112c050566))
* **server:** test for null JSON request body ([7421ccd](https://github.com/supabase-community/scim-go/commit/7421ccd5a0c8d7bd42bcfe51e2d7b052eeef6f27))
* **server:** validate required multi-valued elements attributes ([e4b1ede](https://github.com/supabase-community/scim-go/commit/e4b1ede81ff12c3b9a52497edb028ba8466fa8ef))
* share one value comparator across filter, sort, uniqueness and patch ([6ce6dd9](https://github.com/supabase-community/scim-go/commit/6ce6dd9e752187fc09e480686ee5009aac35dca9))


### Performance Improvements

* **patch:** compile a value filter's literal coercion once per operation ([fd184eb](https://github.com/supabase-community/scim-go/commit/fd184eb1347491bd1fb01fe6c13b8959e6568fa0))
* **patch:** demote other primary values only when a write sets primary ([29c33d7](https://github.com/supabase-community/scim-go/commit/29c33d7b466d151a46cde67735448bc6a4176882))
* **patch:** resolve merged keys through a folded index so PATCH merge is linear ([2e5c835](https://github.com/supabase-community/scim-go/commit/2e5c83596e2a174cd605d6a89814fd06e2dc71da))
* **protocol:** prune the projected document in place ([8217090](https://github.com/supabase-community/scim-go/commit/8217090c949372e24772b081376f1e8deeeabd35))
* **protocol:** skip converting a missing existing resource on decode ([1925546](https://github.com/supabase-community/scim-go/commit/19255462ee307b183fed0aa0e3c3bf7325e961b5))
* **protocol:** strip readOnly request values in place ([c4e4bfb](https://github.com/supabase-community/scim-go/commit/c4e4bfbf1711342146d59533f4563b0d0b18fc5e))
* **server:** check only constrained attributes when validating a write ([ebc37a6](https://github.com/supabase-community/scim-go/commit/ebc37a603594fec38115fd106241b3457c842374))
* **server:** convert a write candidate to a core.Object once for all attribute checks ([a30e60c](https://github.com/supabase-community/scim-go/commit/a30e60c4606f5fcd0c345e47e34c056c05b06a48))
* **server:** index Group member immutability checks by signature instead of scanning candidates ([e5411f2](https://github.com/supabase-community/scim-go/commit/e5411f2745fd4bbb9d1e5cc41145780924399aaa))
* **server:** index the default repository by id and unique values ([4042023](https://github.com/supabase-community/scim-go/commit/4042023566a63326aff3d1c9c91216c382c922ba))


### Reverts

* **server:** keep every Server method in server.go ([f37e26e](https://github.com/supabase-community/scim-go/commit/f37e26e8ff3eddc4aadae1b00b02dc5e850660a6))


### Code Refactoring

* **core:** drop description arg from NewAttribute, add DescribedAs ([29caa92](https://github.com/supabase-community/scim-go/commit/29caa927c4875e08d6049a7418e44067ce57d08f))
* **core:** move basePath from core to server ([03af097](https://github.com/supabase-community/scim-go/commit/03af09751ee89cff2abdb745b8d5a020de742ac3))
* model multi-valued elements and evaluate value paths per element ([b88ef0d](https://github.com/supabase-community/scim-go/commit/b88ef0dc505f662a4101b69713769c02b8c96470))
* **protocol:** pass *protocol.Attribute to Evaluator instead of a key string, extract visitor into visitor.go ([b2445df](https://github.com/supabase-community/scim-go/commit/b2445dfabcab016b0107a7e9cabcc25089e1dec3))
* **server:** build the in-memory repository from explicit arguments ([10a5c68](https://github.com/supabase-community/scim-go/commit/10a5c6823cde25439180a5ebd7babdd20a89da63))

## [0.5.0](https://github.com/supabase-community/scim-go/compare/v0.4.0...v0.5.0) (2026-09-14)


### Features

* **protocol:** add PatchRequest.Apply for RFC 7644 PATCH ([005c15f](https://github.com/supabase-community/scim-go/commit/005c15fcb32ca917dbe3ae75295e08bdc152885d))


### Bug Fixes

* **filter:** make subAttribute production atomic ([acda367](https://github.com/supabase-community/scim-go/commit/acda3674c914b75ceeb7e7fd858182dbe66e910e))
* **patch:** enforce parent mutability on value-path sub-attribute writes ([979c0f7](https://github.com/supabase-community/scim-go/commit/979c0f7a213fe050976133d1bfffc26f583c8162))
* **patch:** match native numerics, fix pr on empty values, remove sub-attr on multi-valued ([81fd5c8](https://github.com/supabase-community/scim-go/commit/81fd5c8f3ba01ce857c32ad8ca296ae3516f58d4))
* **protocol:** enforce parent mutability on value-path merges ([2b9b122](https://github.com/supabase-community/scim-go/commit/2b9b122dd0031c5a531ca7f7dae998eed6483155))
* **protocol:** make PATCH value-path writes schema aware ([c50f21c](https://github.com/supabase-community/scim-go/commit/c50f21c78d7b6bbefd27cb914a9d5de384d19e3e))

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
