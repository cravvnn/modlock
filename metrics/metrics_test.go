package metrics

import (
	"testing"

	"github.com/paralin/modlock/src/wasm/manifest"
	"github.com/paralin/modlock/src/wasm/metrics"
)

// TestValidate checks which declarations a build accepts.
func TestValidate(t *testing.T) {
	// Accept well-formed metrics and refuse each kind of bad one.
	good := &manifest.Manifest{Metrics: []*metrics.Metric{
		{Name: "hud.opened", Kind: metrics.Metric_KIND_COUNT, Labels: []string{"classic", "compact"}},
		{Name: "best_combo", Kind: metrics.Metric_KIND_MAX},
	}}
	if err := Validate(good); err != nil {
		t.Fatalf("Validate(good) = %v", err)
	}
	for name, metric := range map[string]*metrics.Metric{
		"name":  {Name: "9lives", Kind: metrics.Metric_KIND_SUM},
		"kind":  {Name: "runs"},
		"label": {Name: "runs", Kind: metrics.Metric_KIND_SUM, Labels: []string{"two words"}},
		"twice": {Name: "runs", Kind: metrics.Metric_KIND_SUM, Labels: []string{"a", "a"}},
	} {
		if Validate(&manifest.Manifest{Metrics: []*metrics.Metric{metric}}) == nil {
			t.Errorf("Validate accepted a bad %s", name)
		}
	}
	twice := &manifest.Manifest{Metrics: []*metrics.Metric{good.Metrics[1], good.Metrics[1]}}
	if Validate(twice) == nil {
		t.Error("Validate accepted a metric declared twice")
	}
}

// TestTally checks how each kind combines its values.
func TestTally(t *testing.T) {
	// Declare one metric of each kind and check the labels they take.
	opened := &metrics.Metric{Name: "hud.opened", Kind: metrics.Metric_KIND_COUNT, Labels: []string{"compact"}}
	damage := &metrics.Metric{Name: "damage", Kind: metrics.Metric_KIND_SUM}
	combo := &metrics.Metric{Name: "best_combo", Kind: metrics.Metric_KIND_MAX}
	if !Labeled(opened, "compact") || Labeled(opened, "") || !Labeled(damage, "") || Labeled(damage, "compact") {
		t.Fatal("Labeled does not follow the declared labels")
	}

	// Add to each and read the combined totals.
	var tally Tally
	tally.Add(opened, "compact", 5)
	tally.Add(opened, "compact", 5)
	tally.Add(damage, "", 12.5)
	tally.Add(damage, "", 7.5)
	tally.Add(combo, "", -3)
	tally.Add(combo, "", -7)
	if got, want := tally.Text(), "best_combo -3, damage 20, hud.opened[compact] 2"; got != want {
		t.Errorf("Text() = %q, want %q", got, want)
	}
}
