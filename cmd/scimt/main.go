package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
	"text/tabwriter"

	"github.com/supabase-community/scim-go/pkg/scimtest"
)

const (
	childEnv = "SCIMT_CHILD"
	rootName = "Conformance"

	kindPass         = "pass"
	kindMust         = "MUST failure"
	kindShould       = "SHOULD miss"
	kindNA           = "n/a"
	kindGap          = "scimtest gap"
	kindNotRun       = "SHOULD not run"
	kindUnclassified = "unclassified"
	statusPass       = "PASS"
	statusFail       = "FAIL"
	shouldMarker     = "SHOULD: "
	logIndent        = "    "
	sectionDepth     = 3
)

var (
	headerLine = regexp.MustCompile(`^=== (?:RUN|NAME|CONT)\s+(\S+)`)
	statusLine = regexp.MustCompile(`^\s*--- (PASS|FAIL|SKIP): (\S+)`)
	fileLine   = regexp.MustCompile(`^\S+\.go:\d+: `)
)

type result struct {
	name   string
	status string
	logs   []string
}

type tally map[string]map[string]int

type printer struct{ w io.Writer }

func (p printer) say(format string, args ...any) {
	_, _ = fmt.Fprintf(p.w, format, args...)
}

func main() {
	if os.Getenv(childEnv) != "" {
		runSuite()
		return
	}
	os.Exit(runChild())
}

func runSuite() {
	testing.Init()
	url := flag.String("url", os.Getenv("SCIM_URL"), "SCIM base URL, e.g. https://host/scim/v2")
	token := flag.String("token", os.Getenv("SCIM_BEARER_TOKEN"), "bearer token")
	strict := flag.Bool("strict", false, "run SHOULD checks and fail todo skips")
	flag.Parse()
	if *url == "" {
		fmt.Fprintln(os.Stderr, "scimt: -url or SCIM_URL is required, e.g. -url http://localhost:8080/scim/v2")
		os.Exit(2)
	}

	suite := func(t *testing.T) {
		t.Helper()
		opts := []scimtest.Option[*scimtest.Suite]{}
		if *strict {
			opts = append(opts, scimtest.Strict())
		}
		scimtest.NewSuite(scimtest.NewClient(*url, scimtest.WithBearerToken(*token)), opts...).Run(t)
	}
	anything := func(_, _ string) (bool, error) { return true, nil }
	testing.Main(anything, []testing.InternalTest{{Name: rootName, F: suite}}, nil, nil)
}

func runChild() int {
	self, err := os.Executable()
	if err != nil {
		return fail(err)
	}
	//nolint:gosec
	cmd := exec.CommandContext(context.Background(), self, append(os.Args[1:], "-test.v")...)
	cmd.Env = append(os.Environ(), childEnv+"=1")
	cmd.Stderr = os.Stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return fail(err)
	}
	if err := cmd.Start(); err != nil {
		return fail(err)
	}
	results := parse(out)
	_ = cmd.Wait()
	if len(results) == 0 {
		return max(cmd.ProcessState.ExitCode(), 1)
	}
	report(printer{os.Stdout}, results)
	return cmd.ProcessState.ExitCode()
}

func fail(err error) int {
	fmt.Fprintln(os.Stderr, "scimt:", err)
	return 1
}

func parse(r io.Reader) []*result {
	results := []*result{}
	byName := map[string]*result{}
	find := func(name string) *result {
		if byName[name] == nil {
			byName[name] = &result{name: name}
			results = append(results, byName[name])
		}
		return byName[name]
	}
	var current *result
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if m := headerLine.FindStringSubmatch(line); m != nil {
			current = find(m[1])
		} else if m := statusLine.FindStringSubmatch(line); m != nil {
			find(m[2]).status = m[1]
		} else if current != nil && strings.HasPrefix(line, logIndent) {
			current.logs = append(current.logs, fileLine.ReplaceAllString(strings.TrimSpace(line), ""))
		}
	}
	return leaves(results)
}

func leaves(results []*result) []*result {
	found := []*result{}
	for i, r := range results {
		if i+1 == len(results) || !strings.HasPrefix(results[i+1].name, r.name+"/") {
			found = append(found, r)
		}
	}
	return found
}

func classify(r *result) string {
	reason := firstLog(r)
	switch {
	case r.status == statusPass:
		return kindPass
	case r.status == statusFail && hasShould(r):
		return kindShould
	case r.status == statusFail:
		return kindMust
	case strings.HasPrefix(reason, "n/a:"), strings.HasPrefix(reason, "MAY:"):
		return kindNA
	case strings.HasPrefix(reason, "todo:"):
		return kindGap
	case strings.HasPrefix(reason, shouldMarker):
		return kindNotRun
	}
	return kindUnclassified
}

func firstLog(r *result) string {
	if len(r.logs) == 0 {
		return ""
	}
	return r.logs[0]
}

func hasShould(r *result) bool {
	return slices.ContainsFunc(r.logs, func(log string) bool { return strings.HasPrefix(log, shouldMarker) })
}

func sectionOf(name string) string {
	parts := strings.Split(name, "/")
	if len(parts) < sectionDepth {
		return name
	}
	return strings.ReplaceAll(parts[1]+" "+parts[2], "_", " ")
}

func report(out printer, results []*result) {
	counts := tally{}
	byKind := map[string][]*result{}
	for _, r := range results {
		kind, section := classify(r), sectionOf(r.name)
		if counts[section] == nil {
			counts[section] = map[string]int{}
		}
		counts[section][kind]++
		byKind[kind] = append(byKind[kind], r)
	}
	counts.print(out)
	printFailures(out, kindMust, byKind[kindMust])
	printFailures(out, kindShould, byKind[kindShould])
	printReasons(out, byKind[kindNA])
	printResult(out, byKind)
}

func (c tally) print(out printer) {
	sections := make([]string, 0, len(c))
	for section := range c {
		sections = append(sections, section)
	}
	sort.Strings(sections)
	flush := tabwriter.NewWriter(out.w, 0, 0, 2, ' ', 0)
	table := printer{flush}
	table.say("section\tpass\tfail\tn/a\tgap\tnot run\tunclassified\n")
	for _, section := range sections {
		k := c[section]
		table.say("%s\t%d\t%d\t%d\t%d\t%d\t%d\n", section, k[kindPass], k[kindMust]+k[kindShould], k[kindNA], k[kindGap], k[kindNotRun], k[kindUnclassified])
	}
	_ = flush.Flush()
}

func printFailures(out printer, kind string, failures []*result) {
	if len(failures) == 0 {
		return
	}
	out.say("\n%s (%d)\n", kind, len(failures))
	for _, r := range failures {
		out.say("  %s\n", r.name)
		for _, log := range r.logs {
			out.say("    %s\n", log)
		}
	}
}

func printReasons(out printer, skipped []*result) {
	reasons := map[string]int{}
	for _, r := range skipped {
		reasons[firstLog(r)]++
	}
	if len(reasons) == 0 {
		return
	}
	out.say("\nnot applicable (%d)\n", len(skipped))
	names := make([]string, 0, len(reasons))
	for reason := range reasons {
		names = append(names, reason)
	}
	sort.Slice(names, func(i, j int) bool { return reasons[names[i]] > reasons[names[j]] })
	for _, reason := range names {
		out.say("  %3d  %s\n", reasons[reason], reason)
	}
}

func printResult(out printer, byKind map[string][]*result) {
	verdict := "CONFORMANT"
	if len(byKind[kindMust]) > 0 || len(byKind[kindUnclassified]) > 0 {
		verdict = "NOT CONFORMANT"
	}
	out.say("\nResult: %s (%d pass, %d MUST failures, %d SHOULD misses, %d gaps, %d SHOULD not run)\n",
		verdict, len(byKind[kindPass]), len(byKind[kindMust]), len(byKind[kindShould]), len(byKind[kindGap]), len(byKind[kindNotRun]))
}
