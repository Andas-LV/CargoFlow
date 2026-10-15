package stations

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Andas-LV/cargoflow/services/shipment/data"
)

func names(ss []Station) []string {
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		out = append(out, s.NameRu)
	}
	return out
}

func TestSearch(t *testing.T) {
	d := New([]Station{
		{ID: "a", Esr: "677008", Kind: KindStation, Name: "Жезқазған", NameRu: "Джезказган", NameKk: "Жезқазған", NameEn: "Jezqazğan"},
		{ID: "b", Esr: "700007", Kind: KindStation, Name: "Алматы-1", NameRu: "Алматы-1", NameEn: "Almaty-1"},
		{ID: "c", Esr: "700100", Kind: KindStation, Name: "Алматы-2", NameRu: "Алматы-2"},
		{ID: "d", Kind: KindHalt, Name: "Новая Алматы", NameRu: "Новая Алматы"},
		{ID: "e", Kind: KindHalt, Name: "Алматинская", NameRu: "Алматинская"},
	})

	// Russian keyboard finds the Kazakh name.
	assert.Equal(t, []string{"Джезказган"}, names(d.Search("жезказган", 0)))
	// English name and ESR code.
	assert.Equal(t, []string{"Алматы-1"}, names(d.Search("almaty-1", 0)))
	assert.Equal(t, []string{"Джезказган"}, names(d.Search("677008", 0)))
	// Prefix matches first, stations before halts, then word-prefix matches.
	assert.Equal(t, []string{"Алматы-1", "Алматы-2", "Алматинская", "Новая Алматы"}, names(d.Search("алмат", 0)))
	// Limit and empty query.
	assert.Len(t, d.Search("алмат", 2), 2)
	assert.Empty(t, d.Search("  ", 0))

	s, ok := d.Get("b")
	assert.True(t, ok)
	assert.Equal(t, "700007", s.Esr)
}

func TestEmbeddedCSV(t *testing.T) {
	stations, err := ParseCSV(data.StationsKZ)
	require.NoError(t, err)
	assert.Greater(t, len(stations), 1000)

	d := New(stations)
	found := d.Search("Алматы-1", 1)
	require.Len(t, found, 1)
	assert.Equal(t, "700007", found[0].Esr)
	for _, s := range stations {
		assert.NotEmpty(t, s.ID)
		assert.NotEmpty(t, s.NameRu, s.ID)
		assert.Contains(t, []string{KindStation, KindHalt}, s.Kind, s.ID)
	}
}
