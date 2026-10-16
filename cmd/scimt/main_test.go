package main

import (
	"strings"
	"testing"
)

const sample = `=== RUN   Conformance
=== RUN   Conformance/RFC7644
=== RUN   Conformance/RFC7644/3.3_Creating_Resources
=== RUN   Conformance/RFC7644/3.3_Creating_Resources/User
=== RUN   Conformance/RFC7644/3.3_Creating_Resources/User/passes
=== RUN   Conformance/RFC7644/3.3_Creating_Resources/User/skips
    creating.go:9: n/a: nothing to test
=== RUN   Conformance/RFC7644/3.3_Creating_Resources/User/should
    creating.go:10: SHOULD: a rule
    creating.go:11: scimtest: bad
=== RUN   Conformance/RFC7644/3.3_Creating_Resources/User/must
    creating.go:12: scimtest: worse
        --- PASS: Conformance/RFC7644/3.3_Creating_Resources/User/passes (0.00s)
        --- SKIP: Conformance/RFC7644/3.3_Creating_Resources/User/skips (0.00s)
        --- FAIL: Conformance/RFC7644/3.3_Creating_Resources/User/should (0.00s)
        --- FAIL: Conformance/RFC7644/3.3_Creating_Resources/User/must (0.00s)
`

func TestReport(t *testing.T) {
	var out strings.Builder
	report(printer{&out}, parse(strings.NewReader(sample)))

	for _, want := range []string{"RFC7644 3.3 Creating Resources  1     2", "MUST failure (1)", "SHOULD miss (1)", "NOT CONFORMANT (1 pass, 1 MUST failures, 1 SHOULD misses"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("report lacks %q:\n%s", want, out.String())
		}
	}
}
