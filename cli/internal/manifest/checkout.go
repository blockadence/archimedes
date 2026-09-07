package manifest

import "os"

// Checkout is one repo as a command about to work in it needs to see it:
// the repos.yaml entry, its Path resolved against the instance root, and
// whether anything has actually been cloned there.
//
// The two ways that can fall short — a name repos.yaml has no entry for,
// and an entry nothing has cloned yet — are reported apart rather than
// collapsed into one error, because commands react to them differently on
// purpose. A mapping pass skips an uncloned repo and carries on where
// spawn refuses; the ones that refuse each word their own message, some
// naming the path and some the command that would fix it. So this answers
// what is true and leaves both the reaction and the wording to the caller.
type Checkout struct {
	// Repo is the entry as repos.yaml records it, except that Path is
	// resolved against the instance root — so it is usable as a path
	// rather than as the relative one the file carries.
	Repo
	// Listed reports whether repos.yaml carries an entry by that name,
	// and is the field to read first: everything else on an unlisted
	// lookup is zero, including the Path a message might otherwise name.
	Listed bool
	// Cloned reports whether Path is a directory yet. An entry can be
	// listed long before there is a clone at it: bootstrap writes both in
	// one pass, but a hand-added entry, or a checkout the operator has
	// since moved or removed, leaves the two apart.
	Cloned bool
}

// Ready reports whether the checkout is there to be worked in — listed and
// cloned. It is for the callers that treat both shortfalls the same; the
// ones whose messages differ read the two fields instead.
func (c Checkout) Ready() bool { return c.Listed && c.Cloned }

// Checkout looks name up in the instance at root and answers where its
// checkout is and whether it is there — the question every command that
// acts on one named repo asks before it can start.
func (m *Manifest) Checkout(root, name string) Checkout {
	r, ok := m.Find(name)
	if !ok {
		return Checkout{}
	}
	return CheckoutOf(root, r)
}

// CheckoutOf answers the same question about an entry already in hand, for
// a caller walking the whole manifest rather than looking one repo up by
// name.
func CheckoutOf(root string, r Repo) Checkout {
	r = r.resolved(root)
	info, err := os.Stat(r.Path)
	return Checkout{Repo: r, Listed: true, Cloned: err == nil && info.IsDir()}
}
