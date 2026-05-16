package roots

import (
	"testing"
)

func TestSave_Validation(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name    string
		r       Roots
		wantErr bool
	}{
		{
			name:    "empty roots saves fine",
			r:       Roots{Center: "Christ"},
			wantErr: false,
		},
		{
			name: "missing label rejected",
			r: Roots{Nodes: []Node{
				{ID: "a", Ring: RingSpirit, Label: "  "},
			}},
			wantErr: true,
		},
		{
			name: "invalid ring rejected",
			r: Roots{Nodes: []Node{
				{ID: "a", Ring: 99, Label: "child of God"},
			}},
			wantErr: true,
		},
		{
			name: "duplicate id rejected",
			r: Roots{Nodes: []Node{
				{ID: "a", Ring: RingSpirit, Label: "beloved"},
				{ID: "a", Ring: RingMind, Label: "husband"},
			}},
			wantErr: true,
		},
		{
			name: "valid record saves",
			r: Roots{
				Center: "Christ",
				Anchor: "Colossians 1:17",
				Nodes: []Node{
					{ID: "a", Ring: RingSpirit, Label: "beloved son"},
					{ID: "b", Ring: RingMind, Label: "husband"},
				},
			},
			wantErr: false,
		},
	}
	for _, c := range cases {
		err := Save(dir, c.r)
		if (err != nil) != c.wantErr {
			t.Errorf("%s: err=%v wantErr=%v", c.name, err, c.wantErr)
		}
	}
}

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	in := Roots{
		Center: "Christ",
		Anchor: "John 15:5",
		Nodes: []Node{
			{ID: "n1", Ring: RingSpirit, Label: "beloved", Scripture: "Eph 1:4"},
			{ID: "n2", Ring: RingBody, Label: "patience", Description: "given, not earned"},
		},
	}
	if err := Save(dir, in); err != nil {
		t.Fatal(err)
	}
	out := Load(dir)
	if out.Center != in.Center || out.Anchor != in.Anchor {
		t.Errorf("center/anchor mismatch: got %q/%q", out.Center, out.Anchor)
	}
	if len(out.Nodes) != 2 {
		t.Fatalf("got %d nodes, want 2", len(out.Nodes))
	}
	if out.Nodes[0].CreatedAt.IsZero() {
		t.Error("Save should stamp CreatedAt on new nodes")
	}
}

func TestNodesByRing(t *testing.T) {
	r := Roots{Nodes: []Node{
		{ID: "1", Ring: RingSpirit, Label: "a"},
		{ID: "2", Ring: RingMind, Label: "b"},
		{ID: "3", Ring: RingSpirit, Label: "c"},
	}}
	got := r.NodesByRing(RingSpirit)
	if len(got) != 2 {
		t.Fatalf("got %d, want 2", len(got))
	}
	if got[0].Label != "a" || got[1].Label != "c" {
		t.Errorf("order not preserved: %v", got)
	}
}

func TestIsEmpty(t *testing.T) {
	if !(Roots{}).IsEmpty() {
		t.Error("zero Roots should be empty")
	}
	if (Roots{Center: "Christ"}).IsEmpty() {
		t.Error("set center should count as non-empty")
	}
	if (Roots{Nodes: []Node{{Label: "x"}}}).IsEmpty() {
		t.Error("any node should count as non-empty")
	}
}
