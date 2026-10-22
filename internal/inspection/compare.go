package inspection

import (
	"bytes"
	"sort"

	"github.com/deploymenttheory/go-ios-vm/internal/model"
)

// Compare matches exact semantic subject/property identities, never filenames.
// Missing observations remain unexamined; only explicit absence is evidence of absence.
func Compare(left, right []model.Finding) []model.Change {
	l, r := map[string][]model.Observation{}, map[string][]model.Observation{}
	for _, f := range left {
		o := f.Observation
		k := o.Subject + "\x00" + o.Property
		l[k] = append(l[k], o)
	}
	for _, f := range right {
		o := f.Observation
		k := o.Subject + "\x00" + o.Property
		r[k] = append(r[k], o)
	}
	keys := map[string]bool{}
	for k := range l {
		keys[k] = true
	}
	for k := range r {
		keys[k] = true
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	out := make([]model.Change, 0, len(sorted))
	for _, k := range sorted {
		a, b := l[k], r[k]
		c := model.Change{State: "unexamined"}
		if len(a) > 0 {
			c.Left = &a[0]
			c.Subject = a[0].Subject
			c.Property = a[0].Property
		}
		if len(b) > 0 {
			c.Right = &b[0]
			c.Subject = b[0].Subject
			c.Property = b[0].Property
		}
		if len(a) > 1 || len(b) > 1 {
			c.State = "conflict"
		} else if len(a) == 1 && len(b) == 1 {
			c.State = "changed"
			if a[0].Presence == "unexamined" || b[0].Presence == "unexamined" {
				c.State = "unexamined"
			} else if a[0].Presence == b[0].Presence && bytes.Equal(a[0].Value, b[0].Value) {
				c.State = "equal"
			}
		}
		out = append(out, c)
	}
	return out
}
