package combo

// FallbackSelector returns models in the supplied order. It is
// stateless and thread-safe.
type FallbackSelector struct{}

// NextOrder returns a defensive copy of models in input order.
func (f *FallbackSelector) NextOrder(_ string, models []string) []string {
	out := make([]string, len(models))
	copy(out, models)
	return out
}

// Reset is a no-op.
func (f *FallbackSelector) Reset(_ string) {}

// ResetAll is a no-op.
func (f *FallbackSelector) ResetAll() {}
