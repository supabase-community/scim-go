package protocol

type leaf struct {
	key       string
	op        string
	want      any
	caseExact bool
}
