package widgets

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/profiles"
)

// businessPulseWidget renders the business-metrics trend the
// existing dashboard.go displays. The data shape is a sequence
// of (label, value) samples; the widget draws an inline sparkline
// scaled to fit the cell width.
type businessPulseWidget struct{}

func newBusinessPulseWidget() Widget { return &businessPulseWidget{} }

func (businessPulseWidget) ID() string                     { return profiles.WidgetBusinessPulse }
func (businessPulseWidget) Title() string                  { return "Pulse" }
func (businessPulseWidget) MinSize() (int, int)            { return 24, 3 }
func (businessPulseWidget) DataNeeds() []profiles.DataKind { return []profiles.DataKind{profiles.DataBusinessPulse} }

func (w businessPulseWidget) Render(ctx WidgetCtx, width, height int) string {
	if len(ctx.BusinessPulse) == 0 {
		return faintLine("no business metrics — log some in Business Pulse.", width)
	}
	spark := sparkline(ctx.BusinessPulse, width)
	last := ctx.BusinessPulse[len(ctx.BusinessPulse)-1]
	footer := lipgloss.NewStyle().Faint(true).Render(
		fmt.Sprintf("%s: %.1f", last.Label, last.Value))
	return spark + "\n" + footer
}

func (w businessPulseWidget) HandleKey(WidgetCtx, string) (bool, tea.Cmd) {
	return false, nil
}

// sparkline renders the samples as a single-line bar chart
// scaled to width. Min and max determine the height bucket per
// sample; uses 8 unicode block heights.
func sparkline(samples []BusinessSample, width int) string {
	if len(samples) == 0 || width <= 0 {
		return ""
	}
	if len(samples) > width {
		// Downsample by averaging into width buckets.
		bucketSize := float64(len(samples)) / float64(width)
		bucketed := make([]BusinessSample, 0, width)
		for i := 0; i < width; i++ {
			start := int(float64(i) * bucketSize)
			end := int(float64(i+1) * bucketSize)
			if end > len(samples) {
				end = len(samples)
			}
			if start >= end {
				continue
			}
			var sum float64
			for _, s := range samples[start:end] {
				sum += s.Value
			}
			bucketed = append(bucketed, BusinessSample{
				Label: samples[start].Label,
				Value: sum / float64(end-start),
			})
		}
		samples = bucketed
	}
	min, max := samples[0].Value, samples[0].Value
	for _, s := range samples {
		if s.Value < min {
			min = s.Value
		}
		if s.Value > max {
			max = s.Value
		}
	}
	rangeV := max - min
	if rangeV == 0 {
		rangeV = 1
	}
	// 8 height buckets — Unicode block-element characters.
	blocks := []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}
	var b strings.Builder
	for _, s := range samples {
		idx := int((s.Value - min) / rangeV * float64(len(blocks)-1))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(blocks) {
			idx = len(blocks) - 1
		}
		b.WriteRune(blocks[idx])
	}
	return b.String()
}
