package configuration

import "testing"

func listenerFields(diffs []Difference) map[string]bool {
	fields := map[string]bool{}
	for _, difference := range diffs {
		if difference.Scope == NodeManaged {
			fields[difference.Field] = true
		}
	}
	return fields
}

func TestDiffObservedWildcardListenerAcceptsAnyObservedAddresses(t *testing.T) {
	for _, wildcard := range []string{"0.0.0.0", "::"} {
		desired := Document{NodeSpecific: NodeSpecific{BindHosts: []string{wildcard}, DNSPort: 5353}}
		observed := Document{NodeSpecific: NodeSpecific{BindHosts: []string{"10.244.4.137", "127.0.0.1", "::1", "fe80::1%eth0"}, DNSPort: 5353}}
		if fields := listenerFields(DiffObserved(desired, observed)); len(fields) != 0 {
			t.Fatalf("wildcard %q: unexpected listener differences %v", wildcard, fields)
		}
	}
}

func TestDiffObservedWildcardListenerStillComparesPort(t *testing.T) {
	desired := Document{NodeSpecific: NodeSpecific{BindHosts: []string{"0.0.0.0"}, DNSPort: 5353}}
	observed := Document{NodeSpecific: NodeSpecific{BindHosts: []string{"10.244.4.137"}, DNSPort: 53}}
	fields := listenerFields(DiffObserved(desired, observed))
	if !fields["DNS port"] || fields["Bind hosts"] {
		t.Fatalf("expected only a port difference, got %v", fields)
	}
}

func TestDiffObservedWildcardListenerRejectsEmptyObservedAddresses(t *testing.T) {
	desired := Document{NodeSpecific: NodeSpecific{BindHosts: []string{"0.0.0.0"}, DNSPort: 5353}}
	observed := Document{NodeSpecific: NodeSpecific{BindHosts: nil, DNSPort: 5353}}
	if fields := listenerFields(DiffObserved(desired, observed)); !fields["Bind hosts"] {
		t.Fatalf("expected a bind-host difference for an empty observation, got %v", fields)
	}
}

func TestDiffObservedExplicitListenerIsComparedExactly(t *testing.T) {
	desired := Document{NodeSpecific: NodeSpecific{BindHosts: []string{"10.244.6.35", "127.0.0.1"}, DNSPort: 5353}}
	observed := Document{NodeSpecific: NodeSpecific{BindHosts: []string{"10.244.4.137", "127.0.0.1"}, DNSPort: 5353}}
	if fields := listenerFields(DiffObserved(desired, observed)); !fields["Bind hosts"] {
		t.Fatalf("expected a bind-host difference for changed explicit addresses, got %v", fields)
	}
}

func TestDiffObservedWildcardOnObservedSideDoesNotMatchExplicitDesired(t *testing.T) {
	desired := Document{NodeSpecific: NodeSpecific{BindHosts: []string{"192.0.2.10"}, DNSPort: 53}}
	observed := Document{NodeSpecific: NodeSpecific{BindHosts: []string{"0.0.0.0"}, DNSPort: 53}}
	if fields := listenerFields(DiffObserved(desired, observed)); !fields["Bind hosts"] {
		t.Fatalf("expected a bind-host difference, got %v", fields)
	}
}

func TestDiffStillReportsWildcardListenerExactly(t *testing.T) {
	left := Document{NodeSpecific: NodeSpecific{BindHosts: []string{"0.0.0.0"}, DNSPort: 53}}
	right := Document{NodeSpecific: NodeSpecific{BindHosts: []string{"192.0.2.10"}, DNSPort: 53}}
	if fields := listenerFields(Diff(left, right)); !fields["Bind hosts"] {
		t.Fatalf("Diff must stay exact for revision comparisons, got %v", fields)
	}
}
