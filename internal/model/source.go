package model

// Source indicates how a behavior classification was determined.
type Source string

const (
	// SourceHeuristic means the behavior was inferred by the heuristic classifier.
	SourceHeuristic Source = "heuristic"
	// SourceOverride means the behavior was manually specified by the user.
	SourceOverride Source = "override"
)

//nolint:gochecknoglobals // immutable lookup table for source validation
var validSources = map[Source]struct{}{
	SourceHeuristic: {},
	SourceOverride:  {},
}

// IsValid returns true if the source is one of the defined constants.
func (s Source) IsValid() bool {
	_, ok := validSources[s]

	return ok
}
