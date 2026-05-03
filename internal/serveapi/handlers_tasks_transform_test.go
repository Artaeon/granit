package serveapi

import "testing"

// transformText regression: the previous slice index dropped the closing
// "] " from the checkbox prefix, producing lines like "- [ Renamed text"
// that the task parser rejects on rescan — making the task disappear.
// This test pins down the contract for the rewrite.
func TestTransformText_PreservesCheckboxPrefix(t *testing.T) {
	cases := []struct {
		name string
		in   string
		text string
		want string
	}{
		{
			name: "open checkbox plain rename",
			in:   "- [ ] Define the purpose of the widget.",
			text: "Define the purpose of the widget. (renamed)",
			want: "- [ ] Define the purpose of the widget. (renamed)",
		},
		{
			name: "done checkbox preserves x",
			in:   "- [x] old text",
			text: "new text",
			want: "- [x] new text",
		},
		{
			name: "indented checkbox preserves indent",
			in:   "  - [ ] sub",
			text: "sub renamed",
			want: "  - [ ] sub renamed",
		},
		{
			name: "preserves priority + due markers",
			in:   "- [ ] old text !2 due:2026-05-10",
			text: "new text",
			want: "- [ ] new text !2 due:2026-05-10",
		},
		{
			name: "preserves goal + deadline markers",
			in:   "- [ ] old goal:G001 deadline:01abcdefghijklmnopqrstuvwx",
			text: "renamed",
			want: "- [ ] renamed goal:G001 deadline:01abcdefghijklmnopqrstuvwx",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := transformText(tc.in, tc.text)
			if got != tc.want {
				t.Errorf("transformText(%q, %q)\n got: %q\nwant: %q", tc.in, tc.text, got, tc.want)
			}
		})
	}
}
