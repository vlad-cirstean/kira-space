package configsync

// side is one version of a unit: a file's text or an entry's canonical value. ok is false when the
// unit is absent.
type side struct {
	val string
	ok  bool
}

func (a side) equal(b side) bool { return a.ok == b.ok && a.val == b.val }

// verdict is what a three-way decision does to a unit.
type verdict int

const (
	// verdictNone leaves the local unit as it is.
	verdictNone verdict = iota
	// verdictTake writes the shared version, or removes the unit when shared lacks it.
	verdictTake
	// verdictConflict needs the user's choice; the default keeps the local unit.
	verdictConflict
)

// decide is the one three-way rule for files and entries. ours is the local unit, base the shared
// version last applied, theirs the shared version a pull would apply (deactivated units already
// removed). A unit never synced has no base, so a local unit of the same name differs from it.
func decide(ours, base, theirs side) verdict {
	switch {
	case theirs.equal(base):
		return verdictNone
	case ours.equal(base):
		return verdictTake
	case ours.equal(theirs):
		return verdictNone
	}
	return verdictConflict
}

// effective is theirs after a per-unit deactivation: a deactivated unit is absent from shared.
func effective(theirs side, deactivated bool) side {
	if deactivated {
		return side{}
	}
	return theirs
}
