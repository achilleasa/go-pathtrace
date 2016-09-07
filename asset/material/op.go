package material

// OpType represents a blend or modification operator that is applies to
// one or more bxdf expressions.
type OpType uint32

const (
	opInvalid OpType = 10000 + iota
	//
	OpMix
	OpBumpMap
	OpNormalMap
	//
	lastOpEntry
)

// Helper function to check if a value represents an op type.
func IsOpType(t uint32) bool {
	return t > uint32(opInvalid) && t < uint32(lastOpEntry)
}